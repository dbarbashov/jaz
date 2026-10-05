package server

import (
	"context"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/bots"
	"github.com/wins/jaz/backend/internal/storage"
)

func (s *Server) HandleACPTurnFinished(_ context.Context, job acp.Job) {
	status := storage.SessionStatusForACPState(job.State, job.StopReason)
	if job.ID == "" || status == "" {
		return
	}
	session, err := s.Store.LoadSession(job.ID)
	if err != nil {
		s.logger().Error("load completed turn", "session", job.ID, "error", err)
		return
	}
	ids := []string{job.ID}
	if session.ParentID != "" {
		ids = append(ids, session.ParentID)
	}
	if session.Turn != nil && session.Turn.Output != nil && session.Turn.Output.ReplyTo != "" {
		ids = append(ids, session.Turn.Output.ReplyTo)
	}
	unlock := s.lockSession(ids...)
	session, err = s.Store.LoadSession(job.ID)
	var deliveries []storage.TurnDelivery
	if err == nil && session.Turn != nil && status != storage.StatusInterrupted {
		if session.Turn.NotifyParent && session.ParentID != "" {
			parent, loadErr := s.Store.LoadSession(session.ParentID)
			if loadErr != nil {
				err = loadErr
			} else if !parent.Archived && parent.RuntimeRef != nil && parent.RuntimeRef.Agent != acp.AgentJaz {
				deliveries = append(deliveries, storage.TurnDelivery{SessionID: parent.ID, Message: storage.NewInternalQueuedMessage(acp.CompletionPrompt(job))})
			}
		}
		if output := session.Turn.Output; output != nil && output.ReplyTo != "" && len(output.Replies) > 0 {
			deliveries = append(deliveries, storage.TurnDelivery{SessionID: output.ReplyTo, Message: storage.NewInternalQueuedMessage(bots.ReplyPrompt(session.Title, strings.Join(output.Replies, "\n\n")))})
		}
		if err == nil {
			err = s.Store.FinishSessionTurn(job.ID, status, job.Error, time.Now().UTC(), deliveries)
		}
	} else if err == nil && job.State == acp.StateIdle {
		err = s.Store.CompleteSession(job.ID, time.Now().UTC())
	} else if err == nil {
		err = s.Store.UpdateSessionStatus(job.ID, status, job.Error, time.Time{})
	}
	unlock()
	if err != nil {
		s.logger().Error("finish turn and deliver result", "session", job.ID, "error", err)
		_ = s.Store.UpdateSessionStatus(job.ID, storage.StatusInterrupted, err.Error(), time.Time{})
		s.publishSessionChanged(job.ID)
		return
	}
	s.publishMessagesChanged(job.ID)
	s.publishSessionChanged(job.ID)
	for _, delivery := range deliveries {
		s.publishSessionChanged(delivery.SessionID)
		s.drainQueueSoon(delivery.SessionID)
	}
	if job.State == acp.StateIdle {
		s.drainQueueSoon(job.ID)
	}
}

func (s *Server) ResumeQueuedTurns() error {
	sessions, err := s.Store.ListSessions(storage.SessionFilter{Runtime: storage.RuntimeACP, IncludeChildren: true, IncludeSourced: true})
	if err != nil {
		return err
	}
	for _, session := range sessions {
		if session.Status == storage.StatusIdle && len(session.QueuedMessages) > 0 {
			s.drainQueueSoon(session.ID)
		}
	}
	return nil
}

func (s *Server) ResumeInterruptedTurn(ctx context.Context, id string) error {
	session, err := s.Store.LoadSession(id)
	if err != nil {
		return err
	}
	if session.Turn != nil && session.Turn.PendingMessage != nil {
		if session.Turn.PendingMessage.IsAction() {
			go s.runClaimedTurn(context.WithoutCancel(ctx), session, session.Turn.PendingMessage)
			return nil
		}
		return s.startQueuedPrompt(ctx, session, *session.Turn.PendingMessage)
	}
	return s.ACP.ResumeInterruptedTurn(ctx, id)
}
