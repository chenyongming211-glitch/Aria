package grpc

import (
	"context"
	"database/sql"
	"io"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/lib/pq"
	googlegrpc "google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

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
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id, node_public_key, command, params").
		WithArgs(publicKey, controllerstorage.AgentCommandStatusPending).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

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
