package memory

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/code-payments/code-server/pkg/cluster"
)

type Cluster struct {
	ctx    context.Context
	cancel func()

	changeCh chan struct{}

	mu                 sync.Mutex
	memberships        []*Membership
	membershipWatchers []chan []cluster.Member
}

func NewCluster() *Cluster {
	ctx, cancel := context.WithCancel(context.Background())
	c := &Cluster{
		ctx:      ctx,
		cancel:   cancel,
		changeCh: make(chan struct{}),
	}

	go c.watchChanges()

	return c
}

func (c *Cluster) CreateMembership() (cluster.Membership, error) {
	id := uuid.New()

	m := newMembership(c, id.String(), "")
	c.memberships = append(c.memberships, m)
	slices.SortFunc(c.memberships, func(a, b *Membership) int {
		return strings.Compare(a.ID(), b.ID())
	})

	return m, nil
}

func (c *Cluster) Close() {
	c.cancel()
}

func (c *Cluster) GetMembers(_ context.Context) ([]cluster.Member, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.getMembers(), nil
}

func (c *Cluster) WatchMembers(ctx context.Context) <-chan []cluster.Member {
	c.mu.Lock()
	defer c.mu.Unlock()

	ctx, cancel := context.WithCancelCause(ctx)
	go func() {
		select {
		case <-c.ctx.Done():
			cancel(c.ctx.Err())
		case <-ctx.Done():
			cancel(ctx.Err())
		}
	}()

	ch := make(chan []cluster.Member, 1)
	ch <- c.getMembers()
	c.membershipWatchers = append(c.membershipWatchers, ch)

	go func() {
		<-ctx.Done()

		c.mu.Lock()
		defer c.mu.Unlock()

		for i := range c.membershipWatchers {
			if c.membershipWatchers[i] == ch {
				c.membershipWatchers = slices.Delete(c.membershipWatchers, i, i+1)
				break
			}
		}

		close(ch)
	}()

	return ch
}

func (c *Cluster) getMembers() []cluster.Member {
	members := make([]cluster.Member, 0, len(c.memberships))
	for _, m := range c.memberships {
		m.mu.Lock()
		if m.registered {
			members = append(members, m.member)
		}
		m.mu.Unlock()
	}
	return members
}

func (c *Cluster) notify() {
	c.changeCh <- struct{}{}
}

func (c *Cluster) watchChanges() {
	for {
		select {
		case <-c.changeCh:
		case <-c.ctx.Done():
			return
		}

		members := c.getMembers()
		for _, w := range c.membershipWatchers {
			select {
			case w <- members:
			default:
			}
		}
	}
}

type Membership struct {
	c *Cluster

	mu         sync.Mutex
	registered bool
	member     cluster.Member
}

func newMembership(c *Cluster, id string, data string) *Membership {
	return &Membership{
		c: c,
		member: cluster.Member{
			ID:   id,
			Data: data,
		},
	}
}
func (m *Membership) ID() string {
	return m.member.ID
}

func (m *Membership) Data() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.member.Data
}

func (m *Membership) SetData(data string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.member.Data = data

	if m.registered {
		m.c.notify()
	}

	return nil
}

func (m *Membership) Register(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.registered {
		m.registered = true
		m.c.notify()
	}

	return nil
}
func (m *Membership) Deregister(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.registered {
		m.registered = false
		m.c.notify()
	}

	return nil
}
