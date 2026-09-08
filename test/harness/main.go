// Isolated, loopback-only test executable. Never included in the application image.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/endge-lab/service-kit-go/pkg/grpckit"
	"github.com/endge-lab/service-kit-go/pkg/oidc"
	pb "github.com/endge-lab/service-mock-generator/api/mockdata/v1"
	transport "github.com/endge-lab/service-mock-generator/internal/api/grpc/v1"
	"github.com/endge-lab/service-mock-generator/internal/config"
	"github.com/endge-lab/service-mock-generator/internal/platform/schema"
	"github.com/endge-lab/service-mock-generator/internal/usecase/generate"
	"github.com/endge-lab/service-mock-generator/internal/usecase/stream"
	"google.golang.org/grpc"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"syscall"
)

func main() {
	l, e := config.LoadLimits()
	if e != nil {
		log.Fatal(e)
	}
	g := generate.NewUseCase(schema.NewCompiler(), l)
	s := stream.NewUseCase(g, l)
	s.Start()
	defer s.Close()
	verifier, e := oidc.NewVerifier(oidc.VerifierConfig{Issuer: os.Getenv("HARNESS_ISSUER"), JWKSURL: os.Getenv("HARNESS_JWKS"), Audience: "endge-mock-generator", AllowedCallers: []string{"endge-service-backend"}, AllowedAlgorithms: []string{"RS256"}})
	if e != nil {
		log.Fatal(e)
	}
	server := grpc.NewServer(grpc.MaxRecvMsgSize(3<<20), grpc.MaxSendMsgSize(23<<20), grpc.ChainUnaryInterceptor(grpckit.UnaryServerIdentityInterceptor(verifier)), grpc.ChainStreamInterceptor(grpckit.StreamServerIdentityInterceptor(verifier)))
	pb.RegisterMockDataServiceServer(server, transport.NewServer(g, s, l, "0.1.0", "test"))
	listener, e := net.Listen("tcp", "127.0.0.1:"+os.Getenv("HARNESS_GRPC_PORT"))
	if e != nil {
		log.Fatal(e)
	}
	go func() {
		if e := server.Serve(listener); e != nil {
			log.Fatal(e)
		}
	}()
	mux := http.NewServeMux()
	mux.HandleFunc("/goroutines", func(w http.ResponseWriter, r *http.Request) {
		var b bytes.Buffer
		_ = pprof.Lookup("goroutine").WriteTo(&b, 2)
		_, _ = w.Write(b.Bytes())
	})
	mux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		json.NewEncoder(w).Encode(map[string]any{"stream": s.Stats(), "generation": g.Stats(), "heap": m.HeapAlloc, "goroutines": runtime.NumGoroutine()})
	})
	h := &http.Server{Addr: "127.0.0.1:" + os.Getenv("HARNESS_STATS_PORT"), Handler: mux}
	go h.ListenAndServe()
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	<-ctx.Done()
	server.Stop()
	h.Close()
}
