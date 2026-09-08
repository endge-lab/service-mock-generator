package grpcv1_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"github.com/endge-lab/service-kit-go/pkg/grpckit"
	"github.com/endge-lab/service-kit-go/pkg/oidc"
	pb "github.com/endge-lab/service-mock-generator/api/mockdata/v1"
	transport "github.com/endge-lab/service-mock-generator/internal/api/grpc/v1"
	v "github.com/endge-lab/service-mock-generator/internal/domain/valueobjects"
	"github.com/endge-lab/service-mock-generator/internal/platform/schema"
	"github.com/endge-lab/service-mock-generator/internal/usecase/generate"
	"github.com/endge-lab/service-mock-generator/internal/usecase/stream"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNetworkOIDCAndLifecycle(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	var mu sync.Mutex
	active, kid, offline := key, "first", false
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if offline {
			w.WriteHeader(503)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]string{"kty": "RSA", "kid": kid, "alg": "RS256", "n": base64.RawURLEncoding.EncodeToString(active.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(active.E)).Bytes())}}})
	}))
	defer issuer.Close()
	verifier, e := oidc.NewVerifier(oidc.VerifierConfig{Issuer: issuer.URL, JWKSURL: issuer.URL, Audience: "endge-mock-generator", AllowedCallers: []string{"endge-service-backend"}, AllowedAlgorithms: []string{"RS256"}, Timeout: 100 * time.Millisecond})
	if e != nil {
		t.Fatal(e)
	}
	limits := v.DefaultLimits()
	limits.WriteTimeout = 200 * time.Millisecond
	g := generate.NewUseCase(schema.NewCompiler(), limits)
	streams := stream.NewUseCase(g, limits)
	streams.Start()
	defer streams.Close()
	server := grpc.NewServer(grpc.MaxRecvMsgSize(3<<20), grpc.MaxSendMsgSize(23<<20), grpc.ChainUnaryInterceptor(grpckit.UnaryServerIdentityInterceptor(verifier)), grpc.ChainStreamInterceptor(grpckit.StreamServerIdentityInterceptor(verifier)))
	pb.RegisterMockDataServiceServer(server, transport.NewServer(g, streams, limits, "0.1.0", "test"))
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	go server.Serve(listener)
	defer server.Stop()
	conn, e := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(23<<20)))
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Close()
	client := pb.NewMockDataServiceClient(conn)
	signed := func(signing *rsa.PrivateKey, kid, iss, aud, caller string, expires time.Time) context.Context {
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{"iss": iss, "sub": "service-account-backend", "aud": aud, "azp": caller, "exp": expires.Unix()})
		token.Header["kid"] = kid
		value, e := token.SignedString(signing)
		if e != nil {
			t.Fatal(e)
		}
		return metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+value)
	}
	valid := func() context.Context {
		return signed(active, kid, issuer.URL, "endge-mock-generator", "endge-service-backend", time.Now().Add(time.Minute))
	}
	for name, ctx := range map[string]context.Context{
		"missing": context.Background(), "signature": signed(other, "first", issuer.URL, "endge-mock-generator", "endge-service-backend", time.Now().Add(time.Minute)),
		"issuer":   signed(key, "first", "wrong", "endge-mock-generator", "endge-service-backend", time.Now().Add(time.Minute)),
		"audience": signed(key, "first", issuer.URL, "workbench", "endge-service-backend", time.Now().Add(time.Minute)),
		"caller":   signed(key, "first", issuer.URL, "endge-mock-generator", "other", time.Now().Add(time.Minute)),
		"expired":  signed(key, "first", issuer.URL, "endge-mock-generator", "endge-service-backend", time.Now().Add(-time.Minute)),
	} {
		t.Run(name, func(t *testing.T) {
			if _, e := client.GetServiceInfo(ctx, &pb.Empty{}); status.Code(e) != codes.Unauthenticated {
				t.Fatalf("got %v", e)
			}
		})
	}
	if info, e := client.GetServiceInfo(valid(), &pb.Empty{}); e != nil || info.Version != "0.1.0" {
		t.Fatalf("%v %v", info, e)
	}
	mu.Lock()
	active, kid = other, "rotated"
	mu.Unlock()
	if _, e := client.GetServiceInfo(valid(), &pb.Empty{}); e != nil {
		t.Fatal("JWKS rotation", e)
	}
	mu.Lock()
	offline = true
	mu.Unlock()
	if _, e := client.GetServiceInfo(valid(), &pb.Empty{}); e != nil {
		t.Fatal("cached JWKS should survive transient outage", e)
	}
	unknown := signed(other, "unknown", issuer.URL, "endge-mock-generator", "endge-service-backend", time.Now().Add(time.Minute))
	if _, e := client.GetServiceInfo(unknown, &pb.Empty{}); status.Code(e) != codes.Unauthenticated {
		t.Fatal("uncached key accepted during outage", e)
	}
	mu.Lock()
	offline = false
	mu.Unlock()
	owner := &pb.Owner{ActorId: "actor", WorkspaceId: "workspace"}
	result, e := client.Generate(valid(), &pb.GenerateRequest{Owner: owner, Json: []byte(`{"schema":{"$schema":"https://json-schema.org/draft/2020-12/schema","const":9007199254740993},"generation":{"count":1,"seed":"network"}}`)})
	if e != nil {
		t.Fatal(e)
	}
	var raw struct {
		Items []json.RawMessage `json:"items"`
	}
	json.Unmarshal(result.Json, &raw)
	if string(raw.Items[0]) != "9007199254740993" {
		t.Fatalf("number corrupted %s", raw.Items[0])
	}
	prepared, e := client.CreateStream(valid(), &pb.CreateStreamRequest{Owner: owner, Json: []byte(`{"schema":{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"integer"},"generation":{"seed":"network"},"stream":{"intervalMs":100,"itemsPerMessage":1}}`)})
	if e != nil {
		t.Fatal(e)
	}
	var info struct {
		ID string `json:"id"`
	}
	json.Unmarshal(prepared.Json, &info)
	ref := &pb.StreamReference{Owner: owner, Id: info.ID}
	ctx, cancel := context.WithCancel(valid())
	sub, e := client.SubscribeStream(ctx, ref)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = sub.Recv(); e != nil {
		t.Fatal(e)
	}
	if _, e = sub.Recv(); e != nil {
		t.Fatal(e)
	}
	duplicate, _ := client.SubscribeStream(valid(), ref)
	if _, e = duplicate.Recv(); status.Code(e) != codes.FailedPrecondition {
		t.Fatal("duplicate", e)
	}
	foreign := &pb.StreamReference{Owner: &pb.Owner{ActorId: "foreign", WorkspaceId: "workspace"}, Id: info.ID}
	if _, e = client.GetStream(valid(), foreign); status.Code(e) != codes.NotFound {
		t.Fatal("foreign owner", e)
	}
	cancel()
	deadline := time.Now().Add(2 * time.Second)
	for streams.Stats().Sessions != 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if streams.Stats().Sessions != 0 || g.ActiveJobs() != 0 {
		t.Fatal("disconnect leaked work", streams.Stats())
	}
	t.Run("stalled gRPC reader releases write and jobs", func(t *testing.T) {
		raw, _ := json.Marshal(map[string]any{"schema": map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "array", "minItems": 100, "maxItems": 100, "items": map[string]any{"const": strings.Repeat("a", 10000)}}, "generation": map[string]any{"seed": "stalled"}, "stream": map[string]any{"intervalMs": 100, "itemsPerMessage": 1}})
		prepared, e := client.CreateStream(valid(), &pb.CreateStreamRequest{Owner: owner, Json: raw})
		if e != nil {
			t.Fatal(e)
		}
		json.Unmarshal(prepared.Json, &info)
		stalled, e := client.SubscribeStream(valid(), &pb.StreamReference{Owner: owner, Id: info.ID})
		if e != nil {
			t.Fatal(e)
		}
		if _, e = stalled.Recv(); e != nil {
			t.Fatal(e)
		}
		deadline := time.Now().Add(3 * time.Second)
		for streams.Stats().Sessions != 0 && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		stats := streams.Stats()
		if stats.Sessions != 0 || stats.Jobs != 0 || stats.BufferedBytes != 0 || stats.Backpressure == 0 {
			t.Fatal("stalled writer leaked or did not hit backpressure", stats)
		}
	})

}
