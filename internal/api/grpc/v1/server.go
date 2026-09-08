package grpcv1

import (
	"context"
	"encoding/json"
	"errors"
	pb "github.com/endge-lab/service-mock-generator/api/mockdata/v1"
	"github.com/endge-lab/service-mock-generator/internal/domain/entities"
	errs "github.com/endge-lab/service-mock-generator/internal/domain/errors"
	v "github.com/endge-lab/service-mock-generator/internal/domain/valueobjects"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"time"
)

type Generator interface {
	Generate(context.Context, v.Request) (v.Result, error)
}
type Streams interface {
	Create(context.Context, v.Owner, v.StreamRequest) (entities.Stream, error)
	Get(v.Owner, string) (entities.Stream, error)
	Subscribe(context.Context, v.Owner, string, func(context.Context, entities.Event) error) error
	Update(context.Context, v.Owner, string, v.Patch) (entities.Stream, error)
	KeepAlive(v.Owner, string) (entities.Stream, error)
	Stop(v.Owner, string) error
}
type Server struct {
	pb.UnimplementedMockDataServiceServer
	generate     Generator
	streams      Streams
	limits       v.Limits
	version, env string
}

func NewServer(g Generator, s Streams, l v.Limits, version, env string) *Server {
	return &Server{generate: g, streams: s, limits: l, version: version, env: env}
}
func (s *Server) GetServiceInfo(context.Context, *pb.Empty) (*pb.ServiceInfo, error) {
	return &pb.ServiceInfo{Service: "service_mock_generator", Version: s.version, Env: s.env}, nil
}
func (s *Server) GetCapabilities(context.Context, *pb.Empty) (*pb.JSONResponse, error) {
	return reply(map[string]any{"available": true, "protocol": "mockdata.v1", "profiles": []string{"default-v1"}, "limits": s.limits, "idleTimeoutMs": s.limits.IdleTimeout.Milliseconds(), "readyTimeoutMs": s.limits.ReadyTimeout.Milliseconds(), "writeTimeoutMs": s.limits.WriteTimeout.Milliseconds(), "generationTimeoutMs": s.limits.GenerationTimeout.Milliseconds()})
}
func owner(o *pb.Owner) (v.Owner, error) {
	if o == nil || o.ActorId == "" || o.WorkspaceId == "" || len(o.ActorId) > 256 || len(o.WorkspaceId) > 256 {
		return v.Owner{}, status.Error(codes.InvalidArgument, "Owner context is required")
	}
	return v.Owner{ActorID: o.ActorId, WorkspaceID: o.WorkspaceId}, nil
}
func (s *Server) Generate(ctx context.Context, r *pb.GenerateRequest) (*pb.JSONResponse, error) {
	if _, err := owner(r.GetOwner()); err != nil {
		return nil, err
	}
	var req v.Request
	if err := v.Decode(r.GetJson(), &req, s.limits.RequestBytes); err != nil {
		return nil, publicError(err)
	}
	result, err := s.generate.Generate(ctx, req)
	if err != nil {
		return nil, publicError(err)
	}
	return reply(result)
}
func (s *Server) CreateStream(ctx context.Context, r *pb.CreateStreamRequest) (*pb.JSONResponse, error) {
	o, err := owner(r.GetOwner())
	if err != nil {
		return nil, err
	}
	var req v.StreamRequest
	if err = v.Decode(r.GetJson(), &req, s.limits.RequestBytes); err != nil {
		return nil, publicError(err)
	}
	info, err := s.streams.Create(ctx, o, req)
	if err != nil {
		return nil, publicError(err)
	}
	return reply(info)
}
func (s *Server) GetStream(ctx context.Context, r *pb.StreamReference) (*pb.JSONResponse, error) {
	o, err := owner(r.GetOwner())
	if err != nil {
		return nil, err
	}
	info, err := s.streams.Get(o, r.GetId())
	if err != nil {
		return nil, publicError(err)
	}
	return reply(info)
}
func (s *Server) KeepAliveStream(ctx context.Context, r *pb.StreamReference) (*pb.JSONResponse, error) {
	o, err := owner(r.GetOwner())
	if err != nil {
		return nil, err
	}
	info, err := s.streams.KeepAlive(o, r.GetId())
	if err != nil {
		return nil, publicError(err)
	}
	return reply(info)
}
func (s *Server) UpdateStream(ctx context.Context, r *pb.UpdateStreamRequest) (*pb.JSONResponse, error) {
	o, err := owner(r.GetStream().GetOwner())
	if err != nil {
		return nil, err
	}
	var patch v.Patch
	if err = v.Decode(r.GetPatchJson(), &patch, s.limits.RequestBytes); err != nil {
		return nil, publicError(err)
	}
	info, err := s.streams.Update(ctx, o, r.GetStream().GetId(), patch)
	if err != nil {
		return nil, publicError(err)
	}
	return reply(info)
}
func (s *Server) StopStream(ctx context.Context, r *pb.StreamReference) (*pb.Empty, error) {
	o, err := owner(r.GetOwner())
	if err != nil {
		return nil, err
	}
	if err = s.streams.Stop(o, r.GetId()); err != nil {
		return nil, publicError(err)
	}
	return &pb.Empty{}, nil
}
func (s *Server) SubscribeStream(r *pb.StreamReference, out grpc.ServerStreamingServer[pb.StreamEvent]) error {
	o, err := owner(r.GetOwner())
	if err != nil {
		return err
	}
	err = s.streams.Subscribe(out.Context(), o, r.GetId(), func(ctx context.Context, event entities.Event) error {
		raw, err := json.Marshal(event)
		if err != nil {
			return err
		}
		// Returning the handler on timeout closes the RPC and releases a blocked Send.
		done := make(chan error, 1)
		go func() {
			defer func() {
				if recover() != nil {
					done <- errs.Internal("stream.internal", "Stream writer failed")
				}
			}()
			done <- out.Send(&pb.StreamEvent{Json: raw})
		}()
		timer := time.NewTimer(s.limits.WriteTimeout)
		defer timer.Stop()
		select {
		case err := <-done:
			return err
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return errs.New("stream.backpressure", "Stream write deadline exceeded", 429)
		}
	})
	return publicError(err)
}
func reply(value any) (*pb.JSONResponse, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, publicError(err)
	}
	return &pb.JSONResponse{Json: raw}, nil
}
func publicError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return status.Error(codes.Canceled, "Request canceled")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return status.Error(codes.DeadlineExceeded, "Generation deadline exceeded")
	}
	code := codes.Internal
	switch errs.HTTPStatusOf(err) {
	case 400:
		code = codes.InvalidArgument
	case 401:
		code = codes.Unauthenticated
	case 403:
		code = codes.PermissionDenied
	case 404:
		code = codes.NotFound
	case 409:
		code = codes.FailedPrecondition
	case 413, 429:
		code = codes.ResourceExhausted
	case 503:
		code = codes.Unavailable
	case 200:
		code = codes.Canceled
	}
	st := status.New(code, errs.SafeMessageOf(err))
	metadata := map[string]string{}
	if path, ok := errs.DetailsOf(err)["path"].(string); ok {
		metadata["path"] = path
	}
	metadata["httpStatus"] = jsonStatus(errs.HTTPStatusOf(err))
	if detailed, e := st.WithDetails(&errdetails.ErrorInfo{Reason: errs.CodeOf(err), Domain: "mockdata.v1", Metadata: metadata}); e == nil {
		st = detailed
	}
	return st.Err()
}
func jsonStatus(n int) string { b, _ := json.Marshal(n); return string(b) }
