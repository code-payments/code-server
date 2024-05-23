package etcd

import (
	"cmp"
	"context"
	"encoding/json"
	"path"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	v3 "go.etcd.io/etcd/client/v3"

	"github.com/code-payments/code-server/pkg/cluster"
	"github.com/code-payments/code-server/pkg/etcd"
)

type Cluster struct {
	root   string
	client *v3.Client
	ttl    time.Duration

	ctx           context.Context
	cancel        func()
	initializedCh chan struct{}

	mu                 sync.Mutex
	members            []cluster.Member
	membershipWatchers []chan []cluster.Member
}

func NewCluster(client *v3.Client, root string, ttl time.Duration) *Cluster {
	r := &Cluster{
		root:   root,
		client: client,
		ttl:    ttl,

		initializedCh: make(chan struct{}),
	}

	r.ctx, r.cancel = context.WithCancel(context.Background())
	go r.watchMembers()

	// TODO: Maybe this is worth making configurable.
	<-r.initializedCh

	return r
}

func (c *Cluster) CreateMembership() (cluster.Membership, error) {
	id := uuid.New()

	return newMembership(
		c.client,
		c.ttl,
		path.Join(c.root, id.String()),
		id.String(),
		"",
	), nil
}

func (c *Cluster) Close() {
	c.cancel()
}

func (c *Cluster) GetMembers(_ context.Context) ([]cluster.Member, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return slices.Clone(c.members), nil
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
	ch <- c.members
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

func (c *Cluster) watchMembers() {
	watch := etcd.WatchPrefix(
		c.ctx,
		c.client,
		c.root,
		func(_, v []byte) (id string, member cluster.Member, err error) {
			if err = json.Unmarshal(v, &member); err != nil {
				return "", cluster.Member{}, err
			}
			return member.ID, member, nil
		},
	)

	first := true
	for ss := range watch {
		members := make([]cluster.Member, 0)
		for _, v := range ss.Tree {
			members = append(members, v)
		}

		slices.SortFunc(members, func(a, b cluster.Member) int {
			return cmp.Compare(a.ID, b.ID)
		})

		c.mu.Lock()
		c.members = members
		for _, ch := range c.membershipWatchers {
			select {
			case ch <- slices.Clone(members):
			default:
			}
		}
		c.mu.Unlock()

		if first {
			close(c.initializedCh)
			first = false
		}
	}
}

type Membership struct {
	client *v3.Client
	ttl    time.Duration

	key    string
	member cluster.Member

	mu sync.Mutex
	pl *etcd.PersistentLease
}

func newMembership(
	client *v3.Client,
	ttl time.Duration,
	key, id, data string,
) *Membership {
	return &Membership{
		client: client,
		ttl:    ttl,
		key:    key,
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
	b, err := json.Marshal(m.member)
	if err != nil {
		return err
	}

	if pl := m.pl; pl != nil {
		pl.SetValue(string(b))
	}

	return nil
}

func (m *Membership) Register(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.pl != nil {
		return nil
	}

	b, err := json.Marshal(m.member)
	if err != nil {
		return err
	}

	m.pl, err = etcd.NewPersistentLease(
		m.client,
		m.key,
		string(b),
		m.ttl,
	)
	return err
}

func (m *Membership) Deregister(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.pl == nil {
		return nil
	}

	m.pl.Close()
	m.pl = nil

	return nil
}
