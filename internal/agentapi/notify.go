package agentapi

import (
	"sync"

	"github.com/vibemdm/vibemdm/internal/model"
)

// Notifier wakes long-polling agents when work is queued for them and tracks
// on-demand check-in and remote-shell requests.
type Notifier struct {
	mu      sync.Mutex
	subs    map[string][]chan struct{}
	checkin map[string]bool                // computerID -> a check-in has been requested
	shells  map[string][]model.ShellInvite // computerID -> pending shell invites
}

// NewNotifier creates a Notifier.
func NewNotifier() *Notifier {
	return &Notifier{subs: map[string][]chan struct{}{}, checkin: map[string]bool{}, shells: map[string][]model.ShellInvite{}}
}

// Subscribe returns a channel that is closed when the computer is notified; call the returned func to unsubscribe.
func (n *Notifier) Subscribe(computerID string) (<-chan struct{}, func()) {
	ch := make(chan struct{})
	n.mu.Lock()
	n.subs[computerID] = append(n.subs[computerID], ch)
	n.mu.Unlock()
	return ch, func() {
		n.mu.Lock()
		defer n.mu.Unlock()
		subs := n.subs[computerID]
		for i, c := range subs {
			if c == ch {
				n.subs[computerID] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		if len(n.subs[computerID]) == 0 {
			delete(n.subs, computerID)
		}
	}
}

// Notify wakes all waiters for a computer.
func (n *Notifier) Notify(computerID string) {
	n.mu.Lock()
	subs := n.subs[computerID]
	delete(n.subs, computerID)
	n.mu.Unlock()
	for _, ch := range subs {
		close(ch)
	}
}

// RequestCheckin asks the agents to perform a full check-in (policy evaluation)
// as soon as possible. Offline agents pick it up on their next poll.
func (n *Notifier) RequestCheckin(computerIDs ...string) {
	n.mu.Lock()
	for _, id := range computerIDs {
		n.checkin[id] = true
	}
	n.mu.Unlock()
	for _, id := range computerIDs {
		n.Notify(id)
	}
}

// TakeCheckin returns and clears the pending check-in flag.
func (n *Notifier) TakeCheckin(computerID string) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	v := n.checkin[computerID]
	delete(n.checkin, computerID)
	return v
}

// RequestShell queues a shell invite for a computer and wakes its agent.
func (n *Notifier) RequestShell(computerID string, invite model.ShellInvite) {
	n.mu.Lock()
	n.shells[computerID] = append(n.shells[computerID], invite)
	n.mu.Unlock()
	n.Notify(computerID)
}

// TakeShellInvites returns and clears pending shell invites for a computer.
func (n *Notifier) TakeShellInvites(computerID string) []model.ShellInvite {
	n.mu.Lock()
	defer n.mu.Unlock()
	v := n.shells[computerID]
	delete(n.shells, computerID)
	return v
}

// Waiting reports whether an agent is currently long-polling (i.e. online).
func (n *Notifier) Waiting(computerID string) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.subs[computerID]) > 0
}

// OnlineCount returns the number of computers currently long-polling.
func (n *Notifier) OnlineCount() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.subs)
}
