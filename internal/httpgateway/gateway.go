// Package httpgateway is a thin JSON/HTTP shim over the gRPC services for
// the browser console (equivalent to grpc-gateway with
// generate_unbound_methods=true, without the extra codegen toolchain).
//
// It serves POST /<service-full-name>/<MethodName> (e.g.
// POST /auth.v1.PlatformService/DeveloperLogin) with protojson request and
// response bodies (camelCase field names), reusing the generated grpc
// ServiceDesc method handlers so behavior matches the gRPC surface exactly.
package httpgateway

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// maxBodyBytes bounds JSON request bodies (authz model JSON included).
const maxBodyBytes = 1 << 20 // 1 MiB

// Service pairs a generated grpc ServiceDesc with its implementation.
type Service struct {
	Desc *grpc.ServiceDesc
	Impl any
}

type methodEntry struct {
	impl    any
	handler grpc.MethodHandler
}

// Handler routes JSON POSTs to the registered gRPC method handlers.
type Handler struct {
	// methods is keyed by "<service-full-name>/<MethodName>".
	methods     map[string]methodEntry
	interceptor grpc.UnaryServerInterceptor
	allowCORS   bool
}

// New builds the shim. interceptor (may be nil) is applied around every
// call, so rate limiting and logging match the gRPC listener. allowCORS
// enables permissive CORS headers (dev only; the console proxies in dev).
func New(interceptor grpc.UnaryServerInterceptor, allowCORS bool, services ...Service) *Handler {
	h := &Handler{
		methods:     make(map[string]methodEntry),
		interceptor: interceptor,
		allowCORS:   allowCORS,
	}
	for _, svc := range services {
		for i := range svc.Desc.Methods {
			m := &svc.Desc.Methods[i]
			h.methods[svc.Desc.ServiceName+"/"+m.MethodName] = methodEntry{
				impl:    svc.Impl,
				handler: m.Handler,
			}
		}
	}
	return h
}

var marshaler = protojson.MarshalOptions{
	EmitUnpopulated: true, // booleans/zero values always present for the console
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.allowCORS {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}

	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, codes.Unimplemented, "only POST is supported")
		return
	}

	entry, ok := h.methods[strings.Trim(r.URL.Path, "/")]
	if !ok {
		writeError(w, http.StatusNotFound, codes.Unimplemented, "unknown method")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, codes.InvalidArgument, "failed to read request body")
		return
	}
	if len(body) > maxBodyBytes {
		writeError(w, http.StatusRequestEntityTooLarge, codes.InvalidArgument, "request body too large")
		return
	}

	dec := func(req any) error {
		if len(body) == 0 {
			return nil // empty body = empty message (e.g. HealthCheck)
		}
		msg, ok := req.(proto.Message)
		if !ok {
			return fmt.Errorf("request type %T is not a proto message", req)
		}
		if err := protojson.Unmarshal(body, msg); err != nil {
			return status.Error(codes.InvalidArgument, "malformed request body: "+err.Error())
		}
		return nil
	}

	resp, err := entry.handler(entry.impl, r.Context(), dec, h.interceptor)
	if err != nil {
		st := statusFromError(err)
		writeError(w, httpStatus(st.Code()), st.Code(), st.Message())
		return
	}

	msg, ok := resp.(proto.Message)
	if !ok {
		writeError(w, http.StatusInternalServerError, codes.Internal, "unexpected response type")
		return
	}
	out, err := marshaler.Marshal(msg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, codes.Internal, "failed to encode response")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(out); err != nil {
		log.Printf("httpgateway: response write failed: %v", err)
	}
}

func statusFromError(err error) *status.Status {
	if st, ok := status.FromError(err); ok {
		return st
	}
	return status.New(codes.Internal, "internal error")
}

// httpStatus maps the gRPC codes this service actually returns onto HTTP
// (same mapping grpc-gateway uses for these codes).
func httpStatus(code codes.Code) int {
	switch code {
	case codes.InvalidArgument:
		return http.StatusBadRequest
	case codes.Unauthenticated:
		return http.StatusUnauthorized
	case codes.PermissionDenied:
		return http.StatusForbidden
	case codes.NotFound, codes.Unimplemented:
		return http.StatusNotFound
	case codes.ResourceExhausted:
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}

func writeError(w http.ResponseWriter, httpCode int, grpcCode codes.Code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpCode)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code":    int(grpcCode),
		"message": message,
	})
}
