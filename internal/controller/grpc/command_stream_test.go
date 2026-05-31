package grpc

import (
	"context"
	"io"
	"regexp"
	"testing"
	"time"

	"aria/pkg/controllerstorage"
	"aria/pkg/grpc/agentpb"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"google.golang.org/grpc/metadata"
)

type pollingCommandStream struct {
	ctx  context.Context
	recv chan *agentpb.CommandResponse
	sent chan *agentpb.CommandRequest
}

func (s *pollingCommandStream) Recv() (*agentpb.CommandResponse, error) {
	select {
	case msg := <-s.recv:
		if msg == nil {
			return nil, io.EOF
		}
		return msg, nil
	case <-s.ctx.Done():
		return nil, io.EOF
	}
}

func (s *pollingCommandStream) Send(req *agentpb.CommandRequest) error {
	select {
	case s.sent <- req:
		return nil
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

func (s *pollingCommandStream) SetHeader(metadata.MD) error  { return nil }
func (s *pollingCommandStream) SendHeader(metadata.MD) error { return nil }
func (s *pollingCommandStream) SetTrailer(metadata.MD)       {}
func (s *pollingCommandStream) Context() context.Context     { return s.ctx }
func (s *pollingCommandStream) SendMsg(interface{}) error    { return nil }
func (s *pollingCommandStream) RecvMsg(interface{}) error    { return nil }

func TestCommandStreamPollsForCommandsAfterIdleInit(t *testing.T) {
	previousPollInterval := commandStreamPollInterval
	commandStreamPollInterval = 10 * time.Millisecond
	t.Cleanup(func() { commandStreamPollInterval = previousPollInterval })

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	nodeID := uuid.New()
	tenantID := uuid.New()
	publicKey := "pub-key-1"
	commandID := uuid.New().String()
	now := time.Now()

	expectCommandStreamNodeLookup(mock, publicKey, tenantID, nodeID, now)
	expectNoPendingAgentCommand(mock, publicKey)
	expectPendingAgentCommand(mock, publicKey, commandID, now)

	ctx, cancel := context.WithCancel(context.Background())
	ctx = context.WithValue(ctx, RuntimeNodeIDKey, nodeID.String())
	ctx = context.WithValue(ctx, RuntimeTenantIDKey, tenantID.String())
	defer cancel()
	stream := &pollingCommandStream{
		ctx:  ctx,
		recv: make(chan *agentpb.CommandResponse, 1),
		sent: make(chan *agentpb.CommandRequest, 1),
	}
	stream.recv <- &agentpb.CommandResponse{
		CommandId: "init",
		Status:    "ready",
		NodeId:    nodeID.String(),
		PublicKey: publicKey,
	}

	server := &ControllerServer{store: controllerstorage.NewStorageWithDB(db)}
	done := make(chan error, 1)
	go func() {
		done <- server.CommandStream(stream)
	}()

	select {
	case req := <-stream.sent:
		if req.CommandId != commandID {
			t.Fatalf("expected command %s, got %s", commandID, req.CommandId)
		}
		if req.Command != "health_check" {
			t.Fatalf("expected health_check command, got %s", req.Command)
		}
	case <-time.After(time.Second):
		t.Fatalf("expected CommandStream to poll and send command after idle init")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("CommandStream returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatalf("CommandStream did not stop after context cancellation")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet database expectations: %v", err)
	}
}

func expectCommandStreamNodeLookup(mock sqlmock.Sqlmock, publicKey string, tenantID, nodeID uuid.UUID, now time.Time) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE id = $1`)).
		WithArgs(nodeID).
		WillReturnRows(commandStreamNodeRows().AddRow(
			nodeID, publicKey, "machine-1", tenantID, "1.1.1.1:51820", "10.0.0.1", "1.1.1.1", "sh", "vpc-1", "node-1", "100.64.0.2", 2,
			now.Unix(), now.Add(-time.Hour).Unix(), "member", "ebpf", "6.8", true, "online", int64(0), "{}", "", now, now,
		))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE public_key = $1`)).
		WithArgs(publicKey).
		WillReturnRows(commandStreamNodeRows().AddRow(
			nodeID, publicKey, "machine-1", tenantID, "1.1.1.1:51820", "10.0.0.1", "1.1.1.1", "sh", "vpc-1", "node-1", "100.64.0.2", 2,
			now.Unix(), now.Add(-time.Hour).Unix(), "member", "ebpf", "6.8", true, "online", int64(0), "{}", "", now, now,
		))
}

func commandStreamNodeRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "public_key", "machine_id", "tenant_id", "endpoint", "private_ip", "public_ip", "region", "vpc_id", "hostname", "assigned_ip", "ip_offset",
		"last_seen", "registered_at", "role", "runtime_mode", "kernel_version", "has_aesni", "status", "offline_since", "advertised_routes", "enrolled_with_token",
		"created_at", "updated_at",
	})
}

func expectNoPendingAgentCommand(mock sqlmock.Sqlmock, publicKey string) {
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id, node_public_key, command, params, status").
		WithArgs(publicKey, controllerstorage.AgentCommandStatusPending).
		WillReturnRows(sqlmock.NewRows(agentCommandColumns()))
	mock.ExpectRollback()
}

func expectPendingAgentCommand(mock sqlmock.Sqlmock, publicKey, commandID string, now time.Time) {
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id, node_public_key, command, params, status").
		WithArgs(publicKey, controllerstorage.AgentCommandStatusPending).
		WillReturnRows(sqlmock.NewRows(agentCommandColumns()).AddRow(
			commandID,
			publicKey,
			"health_check",
			[]byte(`{}`),
			controllerstorage.AgentCommandStatusPending,
			"",
			5,
			45,
			now,
			now,
			nil,
			nil,
			nil,
			[]byte(`{}`),
		))
	mock.ExpectExec("UPDATE agent_commands").
		WithArgs(commandID, controllerstorage.AgentCommandStatusSent, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

func agentCommandColumns() []string {
	return []string{
		"id", "node_public_key", "command", "params", "status", "message", "priority", "timeout_seconds",
		"created_at", "updated_at", "sent_at", "acknowledged_at", "completed_at", "result",
	}
}
