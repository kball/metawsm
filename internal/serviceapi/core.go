package serviceapi

import (
	"context"
	"sort"
	"strings"

	"metawsm/internal/model"
	"metawsm/internal/orchestrator"
)

type ForumOpenThreadOptions = orchestrator.ForumOpenThreadOptions
type ForumAddPostOptions = orchestrator.ForumAddPostOptions
type ForumAssignThreadOptions = orchestrator.ForumAssignThreadOptions
type ForumChangeStateOptions = orchestrator.ForumChangeStateOptions
type ForumSetPriorityOptions = orchestrator.ForumSetPriorityOptions
type ForumControlSignalOptions = orchestrator.ForumControlSignalOptions
type ForumDebugOptions = orchestrator.ForumDebugOptions
type ForumSearchThreadsOptions = orchestrator.ForumSearchThreadsOptions
type ForumQueueOptions = orchestrator.ForumQueueOptions
type ForumMarkThreadSeenOptions = orchestrator.ForumMarkThreadSeenOptions
type ForumThreadDetail = orchestrator.ForumThreadDetail
type RunSnapshot = orchestrator.RunSnapshot

type Core interface {
	Shutdown()

	ProcessForumBusOnce(ctx context.Context, limit int) (int, error)
	ForumBusHealth() error
	ForumOutboxStats() (model.ForumOutboxStats, error)
	ForumStreamDebugSnapshot(ctx context.Context, options ForumDebugOptions) (model.ForumStreamDebugSnapshot, error)

	RunSnapshot(ctx context.Context, runID string) (RunSnapshot, error)
	ListRunSnapshots(ctx context.Context, ticket string) ([]RunSnapshot, error)

	ForumOpenThread(ctx context.Context, options ForumOpenThreadOptions) (model.ForumThreadView, error)
	ForumAddPost(ctx context.Context, options ForumAddPostOptions) (model.ForumThreadView, error)
	ForumAnswerThread(ctx context.Context, options ForumAddPostOptions) (model.ForumThreadView, error)
	ForumAssignThread(ctx context.Context, options ForumAssignThreadOptions) (model.ForumThreadView, error)
	ForumChangeState(ctx context.Context, options ForumChangeStateOptions) (model.ForumThreadView, error)
	ForumSetPriority(ctx context.Context, options ForumSetPriorityOptions) (model.ForumThreadView, error)
	ForumCloseThread(ctx context.Context, options ForumChangeStateOptions) (model.ForumThreadView, error)
	ForumAppendControlSignal(ctx context.Context, options ForumControlSignalOptions) (model.ForumThreadView, error)

	ForumListThreads(filter model.ForumThreadFilter) ([]model.ForumThreadView, error)
	ForumGetThread(threadID string) (*ForumThreadDetail, error)
	ForumListStats(ticket string, runID string) ([]model.ForumThreadStats, error)
	ForumWatchEvents(ticket string, cursor int64, limit int) ([]model.ForumEvent, error)
	ForumSearchThreads(options ForumSearchThreadsOptions) ([]model.ForumThreadView, error)
	ForumListQueue(options ForumQueueOptions) ([]model.ForumThreadView, error)
	ForumMarkThreadSeen(ctx context.Context, options ForumMarkThreadSeenOptions) (model.ForumThreadSeen, error)
}

type LocalCore struct {
	service *orchestrator.Service
}

func NewLocalCore(dbPath string) (*LocalCore, error) {
	service, err := orchestrator.NewService(dbPath)
	if err != nil {
		return nil, err
	}
	return &LocalCore{service: service}, nil
}

func (l *LocalCore) Shutdown() {
	if l == nil || l.service == nil {
		return
	}
	l.service.Shutdown()
}

func (l *LocalCore) ProcessForumBusOnce(ctx context.Context, limit int) (int, error) {
	return l.service.ProcessForumBusOnce(ctx, limit)
}

func (l *LocalCore) ForumBusHealth() error {
	return l.service.ForumBusHealth()
}

func (l *LocalCore) ForumOutboxStats() (model.ForumOutboxStats, error) {
	return l.service.ForumOutboxStats()
}

func (l *LocalCore) ForumStreamDebugSnapshot(ctx context.Context, options ForumDebugOptions) (model.ForumStreamDebugSnapshot, error) {
	return l.service.ForumStreamDebugSnapshot(ctx, options)
}

func (l *LocalCore) RunSnapshot(ctx context.Context, runID string) (RunSnapshot, error) {
	return l.service.RunSnapshot(ctx, runID)
}

func (l *LocalCore) ListRunSnapshots(ctx context.Context, ticket string) ([]RunSnapshot, error) {
	runs, err := l.service.ListRuns()
	if err != nil {
		return nil, err
	}
	out := make([]RunSnapshot, 0, len(runs))
	ticket = strings.TrimSpace(ticket)
	for _, run := range runs {
		snapshot, err := l.service.RunSnapshot(ctx, strings.TrimSpace(run.RunID))
		if err != nil {
			return nil, err
		}
		if ticket != "" && !snapshotHasTicket(snapshot, ticket) {
			continue
		}
		out = append(out, snapshot)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.TrimSpace(out[i].RunID) > strings.TrimSpace(out[j].RunID)
	})
	return out, nil
}

func (l *LocalCore) ForumOpenThread(ctx context.Context, options ForumOpenThreadOptions) (model.ForumThreadView, error) {
	return l.service.ForumOpenThread(ctx, options)
}

func (l *LocalCore) ForumAddPost(ctx context.Context, options ForumAddPostOptions) (model.ForumThreadView, error) {
	return l.service.ForumAddPost(ctx, options)
}

func (l *LocalCore) ForumAnswerThread(ctx context.Context, options ForumAddPostOptions) (model.ForumThreadView, error) {
	return l.service.ForumAnswerThread(ctx, options)
}

func (l *LocalCore) ForumAssignThread(ctx context.Context, options ForumAssignThreadOptions) (model.ForumThreadView, error) {
	return l.service.ForumAssignThread(ctx, options)
}

func (l *LocalCore) ForumChangeState(ctx context.Context, options ForumChangeStateOptions) (model.ForumThreadView, error) {
	return l.service.ForumChangeState(ctx, options)
}

func (l *LocalCore) ForumSetPriority(ctx context.Context, options ForumSetPriorityOptions) (model.ForumThreadView, error) {
	return l.service.ForumSetPriority(ctx, options)
}

func (l *LocalCore) ForumCloseThread(ctx context.Context, options ForumChangeStateOptions) (model.ForumThreadView, error) {
	return l.service.ForumCloseThread(ctx, options)
}

func (l *LocalCore) ForumAppendControlSignal(ctx context.Context, options ForumControlSignalOptions) (model.ForumThreadView, error) {
	return l.service.ForumAppendControlSignal(ctx, options)
}

func (l *LocalCore) ForumListThreads(filter model.ForumThreadFilter) ([]model.ForumThreadView, error) {
	return l.service.ForumListThreads(filter)
}

func (l *LocalCore) ForumGetThread(threadID string) (*ForumThreadDetail, error) {
	return l.service.ForumGetThread(threadID)
}

func (l *LocalCore) ForumListStats(ticket string, runID string) ([]model.ForumThreadStats, error) {
	return l.service.ForumListStats(ticket, runID)
}

func (l *LocalCore) ForumWatchEvents(ticket string, cursor int64, limit int) ([]model.ForumEvent, error) {
	return l.service.ForumWatchEvents(ticket, cursor, limit)
}

func (l *LocalCore) ForumSearchThreads(options ForumSearchThreadsOptions) ([]model.ForumThreadView, error) {
	return l.service.ForumSearchThreads(options)
}

func (l *LocalCore) ForumListQueue(options ForumQueueOptions) ([]model.ForumThreadView, error) {
	return l.service.ForumListQueue(options)
}

func (l *LocalCore) ForumMarkThreadSeen(ctx context.Context, options ForumMarkThreadSeenOptions) (model.ForumThreadSeen, error) {
	return l.service.ForumMarkThreadSeen(ctx, options)
}

func snapshotHasTicket(snapshot RunSnapshot, ticket string) bool {
	ticket = strings.TrimSpace(ticket)
	for _, candidate := range snapshot.Tickets {
		if strings.EqualFold(strings.TrimSpace(candidate), ticket) {
			return true
		}
	}
	return false
}
