package tdx

import (
	"errors"
	"sync"
)

type Pool struct {
	ch     chan *Client
	mu     sync.Mutex
	closed bool
	dial   func() (*Client, error)
}

func NewPool(dial func() (*Client, error), size int) (*Pool, error) {
	if size <= 0 {
		size = 1
	}
	p := &Pool{
		ch:   make(chan *Client, size),
		dial: dial,
	}
	for i := 0; i < size; i++ {
		c, err := dial()
		if err != nil {
			return nil, err
		}
		p.ch <- c
	}
	return p, nil
}

func (p *Pool) Get() (*Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, errors.New("pool closed")
	}
	select {
	case c := <-p.ch:
		return c, nil
	default:
		return p.dial()
	}
}

func (p *Pool) Put(c *Client) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		c.Close()
		return
	}
	select {
	case p.ch <- c:
	default:
		c.Close()
	}
}

func (p *Pool) Do(fn func(c *Client) error) error {
	c, err := p.Get()
	if err != nil {
		return err
	}
	defer p.Put(c)
	return fn(c)
}

func (p *Pool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.closed = true
	close(p.ch)
	for c := range p.ch {
		c.Close()
	}
}
