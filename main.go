package main

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/coocood/freecache"
	"google.golang.org/grpc"

	"github.com/n0z0/cachedb/cdc"
	"github.com/n0z0/cachedb/proto/cachepb"
)

var (
	version     = "dev"
	showVersion = flag.Bool("version", false, "Tampilkan versi lalu keluar")
	addrFlag    = flag.String("addr", "127.0.0.1:50051", "gRPC address untuk listen (default: 127.0.0.1:50051)")
	portFlag    = flag.String("port", "", "gRPC port untuk listen (contoh: :50051 atau 50051)")
	ttlFlag     = flag.Int("ttl", defaultTTL, "Default TTL data dalam detik (default: 36000 / 10 jam)")

	// CLI inspect flags
	getKey          = flag.String("get", "", "CLI mode: Ambil nilai key dari server CacheDB")
	delKey          = flag.String("del", "", "CLI mode: Hapus key dari server CacheDB")
	setKey          = flag.String("set", "", "CLI mode: Set key ke server CacheDB")
	setVal          = flag.String("val", "", "CLI mode: Nilai untuk -set")
	getStats        = flag.Bool("stats", false, "CLI mode: Tampilkan statistik server CacheDB")
	actorIP         = flag.String("actor", "", "CLI mode: Tampilkan Threat Actor Dossier lengkap untuk sebuah IP")
	listActors      = flag.Bool("actors", false, "CLI mode: Tampilkan daftar semua threat actor aktif di CacheDB")
	exportBlocklist = flag.Bool("blocklist", false, "CLI mode: Ekspor daftar IP ancaman (HIGH/CRITICAL) untuk firewall blocklist")
	exportSTIX      = flag.Bool("export-stix", false, "CLI mode: Ekspor seluruh threat intelligence ke format STIX 2.1 JSON bundle")
	snapshotFlag    = flag.String("snapshot", "cachedb_snapshot.json", "Lokasi file persistence snapshot (kosongkan untuk nonaktifkan)")
	target          = flag.String("target", "127.0.0.1:50051", "Target address server untuk CLI mode")
)

const (
	cacheSizeBytes   = 100 * 1024 * 1024 // 100MB
	defaultTTL       = 36000             // 10 jam (36000 detik)
	maxValueSizeByte = 64 * 1024         // 64KB
)

type actorRegistry struct {
	mu     sync.RWMutex
	actors map[string]time.Time
}

var globalActors = &actorRegistry{
	actors: make(map[string]time.Time),
}

func (r *actorRegistry) registerIP(ip string) {
	if ip == "" || ip == "127.0.0.1" || ip == "::1" {
		return
	}
	r.mu.Lock()
	r.actors[ip] = time.Now()
	r.mu.Unlock()
}

func (r *actorRegistry) removeIP(ip string) {
	r.mu.Lock()
	delete(r.actors, ip)
	r.mu.Unlock()
}

func (r *actorRegistry) allIPs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make([]string, 0, len(r.actors))
	for ip := range r.actors {
		res = append(res, ip)
	}
	return res
}

func maybeTrackActorKey(key string) {
	if net.ParseIP(key) != nil {
		globalActors.registerIP(key)
		return
	}
	if strings.HasPrefix(key, "actor:") || strings.HasPrefix(key, "meta:") {
		idx := strings.LastIndex(key, ":")
		if idx != -1 && idx < len(key)-1 {
			possibleIP := key[idx+1:]
			if net.ParseIP(possibleIP) != nil {
				globalActors.registerIP(possibleIP)
			}
		}
	}
}

type cacheServer struct {
	cache      *freecache.Cache
	defaultTTL int
	cachepb.UnimplementedCacheServer
}

func (s *cacheServer) Get(ctx context.Context, req *cachepb.GetRequest) (*cachepb.GetResponse, error) {
	val, err := s.cache.Get([]byte(req.Key))
	if err != nil {
		return &cachepb.GetResponse{Found: false}, nil
	}
	return &cachepb.GetResponse{
		Value: val,
		Found: true,
	}, nil
}

func (s *cacheServer) Set(ctx context.Context, req *cachepb.SetRequest) (*cachepb.SetResponse, error) {
	if len(req.Value) > maxValueSizeByte {
		return &cachepb.SetResponse{Ok: false}, nil
	}

	ttl := int(req.TtlSeconds)
	if ttl <= 0 {
		ttl = s.defaultTTL
	}

	err := s.cache.Set([]byte(req.Key), req.Value, ttl)
	if err != nil {
		return &cachepb.SetResponse{Ok: false}, nil
	}
	maybeTrackActorKey(req.Key)
	return &cachepb.SetResponse{Ok: true}, nil
}

func (s *cacheServer) Delete(ctx context.Context, req *cachepb.DeleteRequest) (*cachepb.DeleteResponse, error) {
	ok := s.cache.Del([]byte(req.Key))
	return &cachepb.DeleteResponse{Ok: ok}, nil
}

func (s *cacheServer) MGet(ctx context.Context, req *cachepb.MGetRequest) (*cachepb.MGetResponse, error) {
	res := make([]*cachepb.KeyValueItem, len(req.Keys))
	for i, k := range req.Keys {
		val, err := s.cache.Get([]byte(k))
		if err != nil {
			res[i] = &cachepb.KeyValueItem{
				Key:   k,
				Found: false,
			}
		} else {
			res[i] = &cachepb.KeyValueItem{
				Key:   k,
				Value: val,
				Found: true,
			}
		}
	}
	return &cachepb.MGetResponse{Items: res}, nil
}

func (s *cacheServer) MSet(ctx context.Context, req *cachepb.MSetRequest) (*cachepb.MSetResponse, error) {
	var count int32
	for _, item := range req.Items {
		if len(item.Value) > maxValueSizeByte {
			continue
		}
		ttl := int(item.TtlSeconds)
		if ttl <= 0 {
			ttl = s.defaultTTL
		}
		err := s.cache.Set([]byte(item.Key), item.Value, ttl)
		if err == nil {
			count++
			maybeTrackActorKey(item.Key)
		}
	}
	return &cachepb.MSetResponse{Count: count, Ok: true}, nil
}

func (s *cacheServer) GetStats(ctx context.Context, req *cachepb.StatsRequest) (*cachepb.StatsResponse, error) {
	return &cachepb.StatsResponse{
		EntryCount:       s.cache.EntryCount(),
		HitCount:         s.cache.HitCount(),
		MissCount:        s.cache.MissCount(),
		HitRate:          s.cache.HitRate(),
		EvacuateCount:    s.cache.EvacuateCount(),
		ExpiredCount:     s.cache.ExpiredCount(),
		MemoryLimitBytes: cacheSizeBytes,
	}, nil
}

func (s *cacheServer) buildActorDossier(ip string) (*cachepb.ActorDossier, bool) {
	knockPort, _ := s.cache.Get([]byte(ip))
	synHash, _ := s.cache.Get([]byte("actor:syn_hash:" + ip))
	riskScore, _ := s.cache.Get([]byte("actor:risk:" + ip))
	severity, _ := s.cache.Get([]byte("actor:severity:" + ip))
	targetSvc, _ := s.cache.Get([]byte("actor:target_service:" + ip))
	intent, _ := s.cache.Get([]byte("actor:intent:" + ip))
	velocity, _ := s.cache.Get([]byte("actor:velocity:" + ip))
	osName, _ := s.cache.Get([]byte("actor:os:" + ip))
	scanner, _ := s.cache.Get([]byte("actor:scanner:" + ip))
	hits, _ := s.cache.Get([]byte("actor:scan_hits:" + ip))
	lastAct, _ := s.cache.Get([]byte("actor:last_scan:" + ip))
	fileAuthor, _ := s.cache.Get([]byte("meta:author:" + ip))
	fileSoftware, _ := s.cache.Get([]byte("meta:software:" + ip))

	if knockPort == nil && synHash == nil && riskScore == nil && severity == nil &&
		targetSvc == nil && intent == nil && velocity == nil && osName == nil &&
		scanner == nil && hits == nil && lastAct == nil && fileAuthor == nil && fileSoftware == nil {
		return nil, false
	}

	dossier := &cachepb.ActorDossier{
		Ip:             ip,
		KnockPort:      string(knockPort),
		SynHash:        string(synHash),
		RiskScore:      string(riskScore),
		Severity:       string(severity),
		TargetService:  string(targetSvc),
		IntentCategory: string(intent),
		ScanVelocity:   string(velocity),
		EstimatedOs:    string(osName),
		ScannerTool:    string(scanner),
		ScanHits:       string(hits),
		LastActivity:   string(lastAct),
		FileAuthor:     string(fileAuthor),
		FileSoftware:   string(fileSoftware),
	}
	return dossier, true
}

func (s *cacheServer) GetActor(ctx context.Context, req *cachepb.ActorRequest) (*cachepb.ActorResponse, error) {
	dossier, found := s.buildActorDossier(req.Ip)
	if !found {
		return &cachepb.ActorResponse{Found: false}, nil
	}
	return &cachepb.ActorResponse{Found: true, Dossier: dossier}, nil
}

func (s *cacheServer) ListActors(ctx context.Context, req *cachepb.ListActorsRequest) (*cachepb.ListActorsResponse, error) {
	allIPs := globalActors.allIPs()
	var list []*cachepb.ActorDossier

	for _, ip := range allIPs {
		dossier, found := s.buildActorDossier(ip)
		if !found {
			globalActors.removeIP(ip)
			continue
		}
		list = append(list, dossier)
	}

	return &cachepb.ListActorsResponse{
		Actors:     list,
		TotalCount: int32(len(list)),
	}, nil
}

func (s *cacheServer) saveSnapshot(filePath string) {
	if filePath == "" {
		return
	}
	allIPs := globalActors.allIPs()
	var entries []SnapshotEntry

	for _, ip := range allIPs {
		keys := []string{
			ip,
			"actor:syn_hash:" + ip,
			"actor:risk:" + ip,
			"actor:severity:" + ip,
			"actor:target_service:" + ip,
			"actor:intent:" + ip,
			"actor:velocity:" + ip,
			"actor:os:" + ip,
			"actor:scanner:" + ip,
			"actor:scan_hits:" + ip,
			"actor:last_scan:" + ip,
			"meta:author:" + ip,
			"meta:software:" + ip,
		}
		for _, k := range keys {
			val, err := s.cache.Get([]byte(k))
			if err == nil && len(val) > 0 {
				entries = append(entries, SnapshotEntry{
					Key:        k,
					Value:      string(val),
					TTLSeconds: s.defaultTTL,
				})
			}
		}
	}

	snap := CacheSnapshot{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Entries:   entries,
		Actors:    allIPs,
	}

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		log.Printf("[WARN] Gagal serialize snapshot: %v", err)
		return
	}

	tmpFile := filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err == nil {
		_ = os.Rename(tmpFile, filePath)
		log.Printf("[*] Snapshot tersimpan -> %s (%d entries, %d actors)", filePath, len(entries), len(allIPs))
	}
}

func (s *cacheServer) loadSnapshot(filePath string) {
	if filePath == "" {
		return
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return // File tidak ada, wajar saat startup pertama kali
	}

	var snap CacheSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		log.Printf("[WARN] Gagal membaca snapshot %s: %v", filePath, err)
		return
	}

	loaded := 0
	for _, entry := range snap.Entries {
		ttl := entry.TTLSeconds
		if ttl <= 0 {
			ttl = s.defaultTTL
		}
		if err := s.cache.Set([]byte(entry.Key), []byte(entry.Value), ttl); err == nil {
			loaded++
		}
	}
	for _, ip := range snap.Actors {
		globalActors.registerIP(ip)
	}

	log.Printf("[*] Snapshot dimuat dari %s (%d entries, %d actors)", filePath, loaded, len(snap.Actors))
}

type SnapshotEntry struct {
	Key        string `json:"key"`
	Value      string `json:"value"`
	TTLSeconds int    `json:"ttl_seconds"`
}

type CacheSnapshot struct {
	Timestamp string          `json:"timestamp"`
	Entries   []SnapshotEntry `json:"entries"`
	Actors    []string        `json:"actors"`
}

func md5Hex(s string) string {
	h := md5.Sum([]byte(s))
	hexStr := hex.EncodeToString(h[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", hexStr[0:8], hexStr[8:12], hexStr[12:16], hexStr[16:20], hexStr[20:32])
}

func buildSTIXBundle(actors []*cachepb.ActorDossier) map[string]interface{} {
	now := time.Now().UTC().Format(time.RFC3339)
	objects := make([]map[string]interface{}, 0, len(actors))

	for _, a := range actors {
		conf := 50
		if a.RiskScore != "" {
			if sc, err := strconv.Atoi(a.RiskScore); err == nil {
				conf = sc
			}
		}
		desc := fmt.Sprintf("Honeypot probe detected from %s. Tool: %s, OS: %s, SYN-Hash: %s, Service: %s, Risk: %s/100 (%s)",
			a.Ip, a.ScannerTool, a.EstimatedOs, a.SynHash, a.TargetService, a.RiskScore, a.Severity)

		indicator := map[string]interface{}{
			"type":            "indicator",
			"spec_version":    "2.1",
			"id":              fmt.Sprintf("indicator--%s", md5Hex(a.Ip+now)),
			"created":         now,
			"modified":        now,
			"name":            "Malicious Honeypot Probe: " + a.Ip,
			"description":     desc,
			"pattern":         fmt.Sprintf("[ipv4-addr:value = '%s']", a.Ip),
			"pattern_type":    "stix",
			"valid_from":      now,
			"indicator_types": []string{"malicious-activity", "reconnaissance"},
			"confidence":      conf,
			"labels":          []string{a.Severity, a.TargetService, a.ScannerTool},
		}
		objects = append(objects, indicator)
	}

	return map[string]interface{}{
		"type":    "bundle",
		"id":      fmt.Sprintf("bundle--%s", md5Hex(now)),
		"objects": objects,
	}
}

func handleCLIMode() bool {
	if *getKey == "" && *delKey == "" && *setKey == "" && !*getStats && *actorIP == "" && !*listActors && !*exportBlocklist && !*exportSTIX {
		return false
	}

	client, conn, err := cdc.Connect(*target)
	if err != nil {
		log.Fatalf("[CLI] Gagal terhubung ke CacheDB %s: %v", *target, err)
	}
	defer conn.Close()

	if *getStats {
		stats, err := cdc.GetStats(client)
		if err != nil {
			log.Fatalf("[CLI] Gagal mengambil stats: %v", err)
		}
		fmt.Printf("=== CacheDB Server Stats (%s) ===\n", *target)
		fmt.Printf("Total Entries : %d\n", stats.EntryCount)
		fmt.Printf("Hit Count     : %d\n", stats.HitCount)
		fmt.Printf("Miss Count    : %d\n", stats.MissCount)
		fmt.Printf("Hit Rate      : %.2f%%\n", stats.HitRate*100)
		fmt.Printf("Evacuate Count: %d\n", stats.EvacuateCount)
		fmt.Printf("Expired Count : %d\n", stats.ExpiredCount)
		fmt.Printf("Memory Limit  : %d MB\n", stats.MemoryLimitBytes/(1024*1024))
		return true
	}

	if *actorIP != "" {
		dossier, found, err := cdc.GetActor(*actorIP, client)
		if err != nil {
			log.Fatalf("[CLI] Gagal mengambil actor dossier: %v", err)
		}
		if !found {
			fmt.Printf("Threat Actor %q: tidak ditemukan data aktif di CacheDB\n", *actorIP)
			return true
		}
		fmt.Printf("=== THREAT ACTOR DOSSIER: %s ===\n", dossier.Ip)
		if dossier.KnockPort != "" {
			fmt.Printf("Active Knock Port : %s\n", dossier.KnockPort)
		}
		if dossier.RiskScore != "" {
			fmt.Printf("Threat Risk Score : %s / 100 [%s]\n", dossier.RiskScore, dossier.Severity)
		}
		if dossier.TargetService != "" {
			fmt.Printf("Target Service    : %s (Intent: %s)\n", dossier.TargetService, dossier.IntentCategory)
		}
		if dossier.ScannerTool != "" {
			fmt.Printf("Scanner Tool      : %s\n", dossier.ScannerTool)
		}
		if dossier.EstimatedOs != "" {
			fmt.Printf("Estimated OS      : %s\n", dossier.EstimatedOs)
		}
		if dossier.ScanVelocity != "" {
			fmt.Printf("Scan Velocity     : %s\n", dossier.ScanVelocity)
		}
		if dossier.SynHash != "" {
			fmt.Printf("SYN Hash (IOC)    : %s\n", dossier.SynHash)
		}
		if dossier.ScanHits != "" {
			fmt.Printf("Total Probes/Hits : %s\n", dossier.ScanHits)
		}
		if dossier.LastActivity != "" {
			fmt.Printf("Last Activity     : %s\n", dossier.LastActivity)
		}
		if dossier.FileAuthor != "" {
			fmt.Printf("Uploaded Author   : %s\n", dossier.FileAuthor)
		}
		if dossier.FileSoftware != "" {
			fmt.Printf("Uploaded Software : %s\n", dossier.FileSoftware)
		}
		if rdns, err := cdc.Get("actor:rdns:"+*actorIP, client); err == nil && rdns != "" {
			fmt.Printf("Reverse DNS (PTR) : %s\n", rdns)
		}
		if user, err := cdc.Get("actor:classroot:user:"+*actorIP, client); err == nil && user != "" {
			pass, _ := cdc.Get("actor:classroot:pass:"+*actorIP, client)
			fmt.Printf("Harvested Creds   : %s : %s\n", user, pass)
		}
		if gpu, err := cdc.Get("actor:gpu:"+*actorIP, client); err == nil && gpu != "" {
			fmt.Printf("GPU Hardware ID   : %s\n", gpu)
		}
		return true
	}

	if *listActors {
		actors, err := cdc.ListActors(client)
		if err != nil {
			log.Fatalf("[CLI] Gagal list actors: %v", err)
		}
		if len(actors) == 0 {
			fmt.Println("Tidak ada threat actor yang aktif saat ini di CacheDB.")
			return true
		}
		fmt.Printf("=== LIVE ACTIVE THREAT REGISTRY (%d Actors) ===\n", len(actors))
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "IP ADDRESS\tKNOCK\tRISK\tSEVERITY\tSERVICE\tSCANNER TOOL\tOS\tHITS")
		fmt.Fprintln(w, "----------\t-----\t----\t--------\t-------\t------------\t--\t----")
		for _, a := range actors {
			risk := a.RiskScore
			if risk == "" {
				risk = "-"
			}
			sev := a.Severity
			if sev == "" {
				sev = "-"
			}
			port := a.KnockPort
			if port == "" {
				port = "-"
			}
			svc := a.TargetService
			if svc == "" {
				svc = "-"
			}
			tool := a.ScannerTool
			if tool == "" {
				tool = "-"
			}
			osName := a.EstimatedOs
			if osName == "" {
				osName = "-"
			}
			hits := a.ScanHits
			if hits == "" {
				hits = "-"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", a.Ip, port, risk, sev, svc, tool, osName, hits)
		}
		w.Flush()
		return true
	}

	if *exportBlocklist {
		actors, err := cdc.ListActors(client)
		if err != nil {
			log.Fatalf("[CLI] Gagal export blocklist: %v", err)
		}
		count := 0
		for _, a := range actors {
			if a.Severity == "CRITICAL" || a.Severity == "HIGH" || a.RiskScore >= "70" {
				fmt.Println(a.Ip)
				count++
			}
		}
		if count == 0 {
			for _, a := range actors {
				fmt.Println(a.Ip)
			}
		}
		return true
	}

	if *exportSTIX {
		actors, err := cdc.ListActors(client)
		if err != nil {
			log.Fatalf("[CLI] Gagal export STIX: %v", err)
		}
		bundle := buildSTIXBundle(actors)
		data, err := json.MarshalIndent(bundle, "", "  ")
		if err != nil {
			log.Fatalf("[CLI] Gagal serialize STIX JSON: %v", err)
		}
		fmt.Println(string(data))
		return true
	}

	if *getKey != "" {
		val, err := cdc.Get(*getKey, client)
		if err != nil {
			log.Fatalf("[CLI] Gagal get key: %v", err)
		}
		if val == "" {
			fmt.Printf("Key %q: (not found)\n", *getKey)
		} else {
			fmt.Printf("Key %q: %s\n", *getKey, val)
		}
		return true
	}

	if *setKey != "" {
		err := cdc.Set(*setKey, *setVal, client)
		if err != nil {
			log.Fatalf("[CLI] Gagal set key: %v", err)
		}
		fmt.Printf("Key %q berhasil diset: %s\n", *setKey, *setVal)
		return true
	}

	if *delKey != "" {
		err := cdc.Delete(*delKey, client)
		if err != nil {
			log.Fatalf("[CLI] Gagal delete key: %v", err)
		}
		fmt.Printf("Key %q berhasil dihapus\n", *delKey)
		return true
	}

	return true
}

func main() {
	flag.Parse()
	if *showVersion {
		fmt.Println("cachedb", version)
		return
	}

	if handleCLIMode() {
		return
	}

	listenTarget := *addrFlag
	if *portFlag != "" {
		p := *portFlag
		if strings.HasPrefix(p, ":") {
			listenTarget = "127.0.0.1" + p
		} else if !strings.Contains(p, ":") {
			listenTarget = "127.0.0.1:" + p
		} else {
			listenTarget = p
		}
	}

	fc := freecache.NewCache(cacheSizeBytes)

	lis, err := net.Listen("tcp", listenTarget)
	if err != nil {
		log.Fatalf("listen failed on %s: %v", listenTarget, err)
	}

	srv := &cacheServer{cache: fc, defaultTTL: *ttlFlag}
	if *snapshotFlag != "" {
		srv.loadSnapshot(*snapshotFlag)
	}

	s := grpc.NewServer()
	cachepb.RegisterCacheServer(s, srv)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("\n[*] Sinyal shutdown diterima, menyimpan snapshot...")
		if *snapshotFlag != "" {
			srv.saveSnapshot(*snapshotFlag)
		}
		s.GracefulStop()
		os.Exit(0)
	}()

	log.Printf("[*] CacheDB %s server listening on %s (default TTL: %d detik / %.1f jam)", version, listenTarget, *ttlFlag, float64(*ttlFlag)/3600.0)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
