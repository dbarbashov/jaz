package whatsapp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/connections"
	"go.mau.fi/whatsmeow"
	waTypes "go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func TestQRChannelPreservesClientOutcome(t *testing.T) {
	for _, tc := range []struct {
		name    string
		events  []any
		channel whatsmeow.QRChannelItem
		status  string
		err     string
	}{
		{"approved", []any{&events.PairSuccess{}}, whatsmeow.QRChannelSuccess, "scanned", ""},
		{"connected before QR success", []any{&events.PairSuccess{}, &events.Connected{}}, whatsmeow.QRChannelSuccess, "connected", ""},
		{"connected without pair event", []any{&events.Connected{}}, whatsmeow.QRChannelErrUnexpectedEvent, "connected", ""},
		{"connection rejected", []any{&events.ConnectFailure{Reason: events.ConnectFailureServiceUnavailable}}, whatsmeow.QRChannelErrUnexpectedEvent, "failed", "WhatsApp connection failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider, session := newQRTestSession(t)
			jid := waTypes.NewJID("15550101111", waTypes.DefaultUserServer)
			session.client.Store.ID = &jid
			handler := provider.eventHandler(session.client, session)
			for _, event := range tc.events {
				handler(event)
			}
			qrChan := make(chan whatsmeow.QRChannelItem, 1)
			qrChan <- tc.channel
			close(qrChan)
			provider.watchQR(session, qrChan)
			status, err := provider.QRStatus(t.Context(), session.id)
			if err != nil {
				t.Fatal(err)
			}
			if status.Status != tc.status || !strings.Contains(status.Error, tc.err) {
				t.Fatalf("status = %#v", status)
			}
		})
	}
}

func TestQRFailureIsDeliveredBeforeSessionRemoval(t *testing.T) {
	for _, tc := range []struct {
		item   whatsmeow.QRChannelItem
		status string
		err    string
	}{
		{whatsmeow.QRChannelItem{Event: whatsmeow.QRChannelEventPasskeyRequest}, "failed", "requires passkey verification"},
		{whatsmeow.QRChannelItem{Event: whatsmeow.QRChannelEventPasskeyResponse}, "failed", "requires passkey verification"},
		{whatsmeow.QRChannelItem{Event: whatsmeow.QRChannelEventError, Error: errors.New("pairing rejected by WhatsApp")}, "failed", "pairing rejected by WhatsApp"},
		{whatsmeow.QRChannelTimeout, "expired", "closed the pairing socket"},
	} {
		t.Run(tc.item.Event, func(t *testing.T) {
			provider, session := newQRTestSession(t)
			qrChan := make(chan whatsmeow.QRChannelItem, 1)
			qrChan <- tc.item
			close(qrChan)
			provider.watchQR(session, qrChan)
			status, err := provider.QRStatus(t.Context(), session.id)
			if err != nil {
				t.Fatal(err)
			}
			if status.Status != tc.status || !strings.Contains(status.Error, tc.err) {
				t.Fatalf("status = %#v", status)
			}
			if _, err := provider.QRStatus(t.Context(), session.id); !errors.Is(err, connections.ErrQRSessionNotFound) {
				t.Fatalf("terminal session still available: %v", err)
			}
		})
	}
}

func newQRTestSession(t *testing.T) (*Provider, *qrSession) {
	t.Helper()
	provider, err := New(context.Background(), t.TempDir(), Config{}, &fakeWhatsAppStore{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := provider.Close(); err != nil {
			t.Error(err)
		}
	})
	session := &qrSession{
		id:     "whatsapp_qr_test",
		status: "pending",
		ready:  make(chan struct{}),
		client: newWhatsAppClient(provider.container.NewDevice()),
	}
	provider.sessions[session.id] = session
	return provider, session
}
