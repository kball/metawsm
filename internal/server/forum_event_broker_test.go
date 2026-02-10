package server

import (
	"testing"
	"time"

	"metawsm/internal/model"
)

func TestForumEventBrokerSubscribeFiltersByTicketAndRun(t *testing.T) {
	broker := NewForumEventBroker(8)
	t.Cleanup(broker.Close)

	allEvents, closeAll := broker.Subscribe("", "")
	defer closeAll()

	ticketOnly, closeTicket := broker.Subscribe("METAWSM-011", "")
	defer closeTicket()

	ticketAndRun, closeTicketAndRun := broker.Subscribe("METAWSM-011", "run-1")
	defer closeTicketAndRun()

	eventRun1 := model.ForumEvent{
		Sequence: 1,
		Envelope: model.ForumEnvelope{
			Ticket: "METAWSM-011",
			RunID:  "run-1",
		},
	}
	eventRun2 := model.ForumEvent{
		Sequence: 2,
		Envelope: model.ForumEnvelope{
			Ticket: "METAWSM-011",
			RunID:  "run-2",
		},
	}
	eventOtherTicket := model.ForumEvent{
		Sequence: 3,
		Envelope: model.ForumEnvelope{
			Ticket: "METAWSM-999",
			RunID:  "run-1",
		},
	}

	broker.Publish(eventRun1)
	broker.Publish(eventRun2)
	broker.Publish(eventOtherTicket)

	assertReceivesSequences(t, allEvents, []int64{1, 2, 3})
	assertReceivesSequences(t, ticketOnly, []int64{1, 2})
	assertReceivesSequences(t, ticketAndRun, []int64{1})
}

func TestForumEventBrokerUnsubscribeStopsDelivery(t *testing.T) {
	broker := NewForumEventBroker(4)
	t.Cleanup(broker.Close)

	events, unsubscribe := broker.Subscribe("METAWSM-011", "")
	unsubscribe()

	broker.Publish(model.ForumEvent{
		Sequence: 42,
		Envelope: model.ForumEnvelope{
			Ticket: "METAWSM-011",
		},
	})

	select {
	case _, ok := <-events:
		if ok {
			t.Fatalf("expected closed channel after unsubscribe")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timed out waiting for subscriber channel to close")
	}
}

func TestForumEventBrokerDropsStaleMessagesForSlowSubscribers(t *testing.T) {
	broker := NewForumEventBroker(1)
	t.Cleanup(broker.Close)

	events, unsubscribe := broker.Subscribe("METAWSM-011", "")
	defer unsubscribe()

	broker.Publish(model.ForumEvent{
		Sequence: 1,
		Envelope: model.ForumEnvelope{
			Ticket: "METAWSM-011",
		},
	})
	broker.Publish(model.ForumEvent{
		Sequence: 2,
		Envelope: model.ForumEnvelope{
			Ticket: "METAWSM-011",
		},
	})

	select {
	case event := <-events:
		if event.Sequence != 2 {
			t.Fatalf("expected latest sequence 2, got %d", event.Sequence)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timed out waiting for latest event")
	}
}

func assertReceivesSequences(t *testing.T, ch <-chan model.ForumEvent, expected []int64) {
	t.Helper()
	for _, sequence := range expected {
		select {
		case event := <-ch:
			if event.Sequence != sequence {
				t.Fatalf("expected sequence %d, got %d", sequence, event.Sequence)
			}
		case <-time.After(200 * time.Millisecond):
			t.Fatalf("timed out waiting for sequence %d", sequence)
		}
	}
	select {
	case event := <-ch:
		t.Fatalf("unexpected extra event sequence %d", event.Sequence)
	case <-time.After(50 * time.Millisecond):
	}
}
