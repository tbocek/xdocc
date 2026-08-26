package xdocc

import (
	"log"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// pool runs the work of one output file per worker. The walk itself stays
// single-threaded — it renders pages in order and collects them into listings —
// and hands the pool everything that costs: reading a source, minifying it,
// compressing it twice, writing the result out.
type pool struct {
	sem chan struct{}
	wg  sync.WaitGroup

	mu  sync.Mutex
	err error
}

func newPool(workers int) *pool {
	if workers < 1 {
		workers = 1
	}
	return &pool{sem: make(chan struct{}, workers)}
}

// do runs job in the background. It blocks while every worker is busy, so a
// large tree cannot queue up more outstanding work than it can carry: the walk
// runs ahead exactly as far as the pool lets it.
func (p *pool) do(job func() error) {
	p.wg.Add(1)
	p.sem <- struct{}{}
	go func() {
		defer p.wg.Done()
		defer func() { <-p.sem }()
		if err := job(); err != nil {
			p.fail(err)
		}
	}()
}

// fail keeps the first error. The rest of the jobs still run to the end: they
// are already paid for, and stopping halfway would leave the output tree in a
// state cleanup cannot reason about.
func (p *pool) fail(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err == nil {
		p.err = err
	}
}

// wait blocks until every job is finished and returns the first error.
func (p *pool) wait() error {
	p.wg.Wait()
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.err
}

// defaultWorkers is how many output files xdocc works on at once. Minifying and
// compressing at the highest setting is processor-bound, and the one worker
// above the processor count covers the moments one of them is waiting on the
// disk instead of working.
func defaultWorkers() int { return runtime.NumCPU() + 1 }

// Workers is how many output files are minified, compressed and written at
// once. Defaults to one more than the number of processors; "workers: 1" in
// the root .xdocc makes a build single-threaded, which is what to reach for
// when a build has to be reproduced or a log read in order. Site-wide, read
// from the root .xdocc only.
func (s *Site) Workers() int {
	if s.Root == nil {
		return defaultWorkers()
	}
	raw, ok := s.Root.Props[PropWorkers]
	if !ok {
		return defaultWorkers()
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 1 {
		log.Printf("xdocc: %s: %q is not a number of workers, using %d", PropWorkers, raw, defaultWorkers())
		return defaultWorkers()
	}
	return n
}
