package memory

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/code-payments/code-server/pkg/cluster"
)

// Cluster maintains a view of the cluster state.
// Members _controL_ their own membership. lock control should be on the internal

type Cluster struct {
	changeCh chan struct{}
	closeCh  chan struct{}
	closeFn  sync.Once

	membersMu sync.Mutex
	members   []cluster.Member

	watchersMu sync.Mutex
	watchers   []chan []cluster.Member
}

func NewCluster() *Cluster {
	c := &Cluster{
		changeCh: make(chan struct{}),
		closeCh:  make(chan struct{}),
	}

	go c.watchChanges()

	return c
}

func (c *Cluster) CreateMembership() (cluster.Membership, error) {
	return &Member{c: c, id: uuid.New().String()}, nil
}

func (c *Cluster) GetMembers(_ context.Context) ([]cluster.Member, error) {
	return c.cloneMembers(), nil
}

func (c *Cluster) WatchMembers(ctx context.Context) <-chan []cluster.Member {
	ch := make(chan []cluster.Member, 1)

	c.watchersMu.Lock()
	c.watchers = append(c.watchers, ch)
	members := c.cloneMembers()
	ch <- members
	c.watchersMu.Unlock()

	// Question: when you call cancel() it's just super vague...you're not 'declaring' it, per se.

	go func() {
		defer func() {
			c.watchersMu.Lock()
			defer c.watchersMu.Unlock()

			for i := range c.watchers {
				if c.watchers[i] == ch {
					c.watchers = slices.Delete(c.watchers, i, i+1)
					break
				}
			}

			close(ch)
		}()

		select {
		case <-ctx.Done():
		case <-c.closeCh:
		}
	}()

	return ch
}

func (c *Cluster) Close() {
	c.closeFn.Do(func() {
		close(c.closeCh)
	})
}

func (c *Cluster) cloneMembers() []cluster.Member {
	c.membersMu.Lock()
	defer c.membersMu.Unlock()

	members := make([]cluster.Member, len(c.members))
	copy(members, c.members)
	return members
}

func (c *Cluster) notify() {
	select {
	case <-c.closeCh:
	case c.changeCh <- struct{}{}:
	}
}

func (c *Cluster) watchChanges() {
	for {
		select {
		case <-c.closeCh:
			return
		case <-c.changeCh:
		}

		latest := c.cloneMembers()

		// When we want to _write_, _add_, or _remove to any watchers,
		// We appear to have a race, where we want to Remove(), but are in the middle of a write.
		// This is actually caused by the fact that Write() is blocking, because the downstream
		// consumer is not consuming. Why?

		c.watchersMu.Lock()
		for _, ch := range c.watchers {
			ch <- latest
		}
		c.watchersMu.Unlock()
	}
}

type Member struct {
	c  *Cluster
	id string

	sync.Mutex
	data       string
	registered bool
}

func (m *Member) ID() string {
	return m.id
}

func (m *Member) Data() string {
	return m.data
}

func (m *Member) SetData(data string) error {
	m.data = data
	if !m.registered {
		return nil
	}

	m.c.membersMu.Lock()
	for i := range m.c.members {
		if m.c.members[i].ID == m.id {
			m.c.members[i].Data = data
			break
		}
	}
	m.c.membersMu.Unlock()

	m.c.notify()

	return nil
}

func (m *Member) Register(ctx context.Context) error {
	if m.registered {
		return nil
	}

	m.registered = true

	m.c.membersMu.Lock()
	m.c.members = append(m.c.members, cluster.Member{ID: m.id, Data: m.data})
	slices.SortFunc(m.c.members, func(a, b cluster.Member) int {
		return strings.Compare(a.ID, b.ID)
	})
	m.c.membersMu.Unlock()

	m.c.notify()

	return nil
}

func (m *Member) Deregister(ctx context.Context) error {
	if !m.registered {
		return nil
	}

	m.registered = false

	m.c.membersMu.Lock()
	for i := range m.c.members {
		if m.c.members[i].ID == m.id {
			m.c.members = slices.Delete(m.c.members, i, i+1)
			break
		}
	}
	m.c.membersMu.Unlock()

	m.c.notify()

	return nil
}
