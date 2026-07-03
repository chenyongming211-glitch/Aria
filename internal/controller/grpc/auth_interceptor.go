package grpc

import (
	"context"
	"strings"

	"aria/internal/auth"
	controllerstorage "aria/pkg/controllerstorage"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type contextKey string

const (
	RuntimeNodeIDKey   contextKey = "runtime_node_id"
	RuntimeTenantIDKey contextKey = "runtime_tenant_id"
)

func GetRuntimeNodeID(ctx context.Context) (string, bool) {
	val := ctx.Value(RuntimeNodeIDKey)
	if val == nil {
		return "", false
	}
	nodeID, ok := val.(string)
	return nodeID, ok
}

func GetRuntimeTenantID(ctx context.Context) (string, bool) {
	val := ctx.Value(RuntimeTenantIDKey)
	if val == nil {
		return "", false
	}
	tenantID, ok := val.(string)
	return tenantID, ok
}

func extractBearerToken(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Error(codes.Unauthenticated, "missing metadata")
	}

	values := md.Get("authorization")
	if len(values) == 0 {
		return "", status.Error(codes.Unauthenticated, "missing authorization header")
	}

	authHeader := strings.TrimSpace(values[0])
	if !strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		return "", status.Error(codes.Unauthenticated, "invalid authorization format")
	}
	token := strings.TrimSpace(authHeader[len("bearer "):])
	if token == "" {
		return "", status.Error(codes.Unauthenticated, "invalid authorization format")
	}

	return token, nil
}

func validateRuntimeTokenState(store *controllerstorage.Storage, claims *auth.RuntimeClaims) error {
	if store == nil {
		return status.Errorf(codes.Internal, "runtime auth store unavailable")
	}
	if claims == nil {
		return status.Errorf(codes.Unauthenticated, "invalid runtime token")
	}
	parsedNodeID, err := uuid.Parse(strings.TrimSpace(claims.NodeID))
	if err != nil {
		return status.Errorf(codes.Unauthenticated, "invalid node_id in token")
	}
	parsedTenantID, err := uuid.Parse(strings.TrimSpace(claims.TenantID))
	if err != nil {
		return status.Errorf(codes.Unauthenticated, "invalid tenant_id in token")
	}

	state, err := store.GetNodeRuntimeAuthState(parsedNodeID)
	if err != nil || state == nil {
		return status.Errorf(codes.Unauthenticated, "node not found")
	}
	if state.TenantID != parsedTenantID {
		return status.Errorf(codes.PermissionDenied, "runtime token tenant mismatch")
	}
	if claims.TokenVersion != state.RuntimeTokenVersion {
		return status.Errorf(codes.Unauthenticated, "runtime token revoked")
	}

	switch state.NodeStatus {
	case "deleted", "suspended", "banned":
		return status.Errorf(codes.PermissionDenied, "node access denied: status '%s'", state.NodeStatus)
	}
	if !strings.EqualFold(strings.TrimSpace(state.TenantStatus), "active") {
		return status.Errorf(codes.PermissionDenied, "tenant is not active: status '%s'", state.TenantStatus)
	}

	return nil
}

func UnaryAuthInterceptor(store *controllerstorage.Storage) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		if isRegisterMethod(info.FullMethod) {
			return handler(ctx, req)
		}

		token, err := extractBearerToken(ctx)
		if err != nil {
			return nil, err
		}

		claims, err := auth.ValidateRuntimeToken(token)
		if err != nil {
			if err == auth.ErrExpiredToken {
				return nil, status.Error(codes.Unauthenticated, "runtime token expired")
			}
			return nil, status.Error(codes.Unauthenticated, "invalid runtime token")
		}

		if err := validateRuntimeTokenState(store, claims); err != nil {
			return nil, err
		}

		ctx = context.WithValue(ctx, RuntimeNodeIDKey, claims.NodeID)
		ctx = context.WithValue(ctx, RuntimeTenantIDKey, claims.TenantID)

		return handler(ctx, req)
	}
}

func StreamAuthInterceptor(store *controllerstorage.Storage) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		if isRegisterMethod(info.FullMethod) {
			return handler(srv, ss)
		}

		token, err := extractBearerToken(ss.Context())
		if err != nil {
			return err
		}

		claims, err := auth.ValidateRuntimeToken(token)
		if err != nil {
			if err == auth.ErrExpiredToken {
				return status.Error(codes.Unauthenticated, "runtime token expired")
			}
			return status.Error(codes.Unauthenticated, "invalid runtime token")
		}

		if err := validateRuntimeTokenState(store, claims); err != nil {
			return err
		}

		ctx := context.WithValue(ss.Context(), RuntimeNodeIDKey, claims.NodeID)
		ctx = context.WithValue(ctx, RuntimeTenantIDKey, claims.TenantID)

		wrappedStream := &wrappedServerStream{ServerStream: ss, ctx: ctx}
		return handler(srv, wrappedStream)
	}
}

type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}

func isRegisterMethod(fullMethod string) bool {
	return fullMethod == "/aria.agent.ControllerService/Register"
}
