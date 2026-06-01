package grpc

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/lib/pq"
	googlegrpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	controllerstorage "aria/pkg/controllerstorage"
	"aria/pkg/grpc/agentpb"
)

func TestCommandStreamSendsHeaderAfterInitWithoutPendingCommand(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	const publicKey = "agent-public-key"
	nodeID := uuid.New()
	tenantID := uuid.New()
	now := time.Now()

	mock.ExpectQuery("SELECT id, public_key, machine_id, tenant_id").
		WithArgs(publicKey).
		WillReturnRows(sqlmock.NewRows(nodeRowColumns()).AddRow(
			nodeID, publicKey, "machine-1", tenantID,
			"1.2.3.4:51820", "", "1.2.3.4", "test-region",
			"", "node-1", "100.64.0.2", 2,
			now.Unix(), now.Unix(), "agent",
			"ebpf", "6.8.0", true,
			"online", int64(0), pq.StringArray{},
			"", now, now,
		))
	mock.ExpectQuery("SELECT id, public_key, machine_id, tenant_id").
		WithArgs(publicKey).
		WillReturnRows(sqlmock.NewRows(nodeRowColumns()).AddRow(
			nodeID, publicKey, "machine-1", tenantID,
			"1.2.3.4:51820", "", "1.2.3.4", "test-region",
			"", "node-1", "100.64.0.2", 2,
			now.Unix(), now.Unix(), "agent",
			"ebpf", "6.8.0", true,
			"online", int64(0), pq.StringArray{},
			"", now, now,
		))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE agent_commands").
		WithArgs(publicKey, controllerstorage.AgentCommandStatusPending, controllerstorage.AgentCommandStatusSent).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT id, node_public_key, command, params").
		WithArgs(publicKey, controllerstorage.AgentCommandStatusPending).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	ctx := context.WithValue(context.Background(), RuntimeNodeIDKey, nodeID.String())
	ctx = context.WithValue(ctx, RuntimeTenantIDKey, tenantID.String())

	stream := &fakeCommandStream{
		recv: []*agentpb.CommandResponse{{
			CommandId: "init",
			Status:    "ready",
			Message:   "agent connected",
			Result: map[string]string{
				"public_key": publicKey,
			},
			PublicKey: publicKey,
		}},
		ctx: ctx,
	}

	server := NewControllerServer(nil, nil, controllerstorage.NewStorageWithDB(db))
	if err := server.CommandStream(stream); err != nil {
		t.Fatalf("CommandStream returned error: %v", err)
	}

	if stream.sendHeaderCalls == 0 {
		t.Fatal("CommandStream did not send initial headers after init")
	}
	if len(stream.sent) != 0 {
		t.Fatalf("expected no command requests, got %d", len(stream.sent))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestCommandStreamRequeuesCommandWhenSendFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	const publicKey = "agent-public-key"
	commandID := uuid.New().String()
	nodeID := uuid.New()
	tenantID := uuid.New()
	now := time.Now()

	mock.ExpectQuery("SELECT id, public_key, machine_id, tenant_id").
		WithArgs(publicKey).
		WillReturnRows(sqlmock.NewRows(nodeRowColumns()).AddRow(
			nodeID, publicKey, "machine-1", tenantID,
			"1.2.3.4:51820", "", "1.2.3.4", "test-region",
			"", "node-1", "100.64.0.2", 2,
			now.Unix(), now.Unix(), "agent",
			"ebpf", "6.8.0", true,
			"online", int64(0), pq.StringArray{},
			"", now, now,
		))
	mock.ExpectQuery("SELECT id, public_key, machine_id, tenant_id").
		WithArgs(publicKey).
		WillReturnRows(sqlmock.NewRows(nodeRowColumns()).AddRow(
			nodeID, publicKey, "machine-1", tenantID,
			"1.2.3.4:51820", "", "1.2.3.4", "test-region",
			"", "node-1", "100.64.0.2", 2,
			now.Unix(), now.Unix(), "agent",
			"ebpf", "6.8.0", true,
			"online", int64(0), pq.StringArray{},
			"", now, now,
		))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE agent_commands").
		WithArgs(publicKey, controllerstorage.AgentCommandStatusPending, controllerstorage.AgentCommandStatusSent).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT id, node_public_key, command, params").
		WithArgs(publicKey, controllerstorage.AgentCommandStatusPending).
		WillReturnRows(sqlmock.NewRows(agentCommandColumns()).AddRow(
			commandID, publicKey, "health_check", []byte(`{}`), controllerstorage.AgentCommandStatusPending,
			"", 5, 45, now, now, nil, nil, nil, []byte(`{}`),
		))
	mock.ExpectExec("UPDATE agent_commands").
		WithArgs(commandID, controllerstorage.AgentCommandStatusSent, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectExec("UPDATE agent_commands").
		WithArgs(commandID, publicKey, controllerstorage.AgentCommandStatusPending, "stream send failed: boom", controllerstorage.AgentCommandStatusSent).
		WillReturnResult(sqlmock.NewResult(0, 1))

	ctx := context.WithValue(context.Background(), RuntimeNodeIDKey, nodeID.String())
	ctx = context.WithValue(ctx, RuntimeTenantIDKey, tenantID.String())
	stream := &fakeCommandStream{
		recv: []*agentpb.CommandResponse{{
			CommandId: "init",
			Status:    "ready",
			PublicKey: publicKey,
		}},
		ctx:     ctx,
		sendErr: errors.New("boom"),
	}

	server := NewControllerServer(nil, nil, controllerstorage.NewStorageWithDB(db))
	if err := server.CommandStream(stream); err == nil {
		t.Fatal("expected CommandStream to fail when sending command fails")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestCommandStreamStopsWhenNodeSuspendedMidStream(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	const publicKey = "agent-public-key"
	nodeID := uuid.New()
	tenantID := uuid.New()
	now := time.Now()

	mock.ExpectQuery("SELECT id, public_key, machine_id, tenant_id").
		WithArgs(publicKey).
		WillReturnRows(sqlmock.NewRows(nodeRowColumns()).AddRow(
			nodeID, publicKey, "machine-1", tenantID,
			"1.2.3.4:51820", "", "1.2.3.4", "test-region",
			"", "node-1", "100.64.0.2", 2,
			now.Unix(), now.Unix(), "agent",
			"ebpf", "6.8.0", true,
			"online", int64(0), pq.StringArray{},
			"", now, now,
		))
	mock.ExpectQuery("SELECT id, public_key, machine_id, tenant_id").
		WithArgs(publicKey).
		WillReturnRows(sqlmock.NewRows(nodeRowColumns()).AddRow(
			nodeID, publicKey, "machine-1", tenantID,
			"1.2.3.4:51820", "", "1.2.3.4", "test-region",
			"", "node-1", "100.64.0.2", 2,
			now.Unix(), now.Unix(), "agent",
			"ebpf", "6.8.0", true,
			"suspended", int64(0), pq.StringArray{},
			"", now, now,
		))

	ctx := context.WithValue(context.Background(), RuntimeNodeIDKey, nodeID.String())
	ctx = context.WithValue(ctx, RuntimeTenantIDKey, tenantID.String())
	stream := &fakeCommandStream{
		recv: []*agentpb.CommandResponse{{
			CommandId: "init",
			Status:    "ready",
			PublicKey: publicKey,
		}},
		ctx: ctx,
	}

	server := NewControllerServer(nil, nil, controllerstorage.NewStorageWithDB(db))
	err = server.CommandStream(stream)
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied when node is suspended mid-stream, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func nodeRowColumns() []string {
	return []string{
		"id", "public_key", "machine_id", "tenant_id",
		"endpoint", "private_ip", "public_ip", "region",
		"vpc_id", "hostname", "assigned_ip", "ip_offset",
		"last_seen", "registered_at", "role",
		"runtime_mode", "kernel_version", "has_aesni",
		"status", "offline_since", "advertised_routes",
		"enrolled_with_token", "created_at", "updated_at",
	}
}

type fakeCommandStream struct {
	recv            []*agentpb.CommandResponse
	sent            []*agentpb.CommandRequest
	sendHeaderCalls int
	ctx             context.Context
	sendErr         error
}

var _ agentpb.ControllerService_CommandStreamServer = (*fakeCommandStream)(nil)

func (s *fakeCommandStream) Recv() (*agentpb.CommandResponse, error) {
	if len(s.recv) == 0 {
		return nil, io.EOF
	}
	next := s.recv[0]
	s.recv = s.recv[1:]
	return next, nil
}

func (s *fakeCommandStream) Send(req *agentpb.CommandRequest) error {
	if s.sendErr != nil {
		return s.sendErr
	}
	s.sent = append(s.sent, req)
	return nil
}

func (s *fakeCommandStream) SetHeader(metadata.MD) error {
	return nil
}

func (s *fakeCommandStream) SendHeader(metadata.MD) error {
	s.sendHeaderCalls++
	return nil
}

func (s *fakeCommandStream) SetTrailer(metadata.MD) {}

func (s *fakeCommandStream) Context() context.Context {
	if s.ctx != nil {
		return s.ctx
	}
	return context.Background()
}

func (s *fakeCommandStream) SendMsg(any) error {
	return nil
}

func (s *fakeCommandStream) RecvMsg(any) error {
	return nil
}

func (s *fakeCommandStream) ServerStream() googlegrpc.ServerStream {
	return s
}
