package store

import (
	"context"
)

type Store struct {
	ctx           context.Context
	localChan     chan *LocalArbitrage
	committedChan chan *CommittedArbitrage
	executedChan  chan *ExecutedArbitrage
	dao           *Dao
}

func NewStore(ctx context.Context, url, scheme, user, passwd string) *Store {
	s := &Store{
		ctx:           ctx,
		localChan:     make(chan *LocalArbitrage, 32),
		committedChan: make(chan *CommittedArbitrage, 32),
		executedChan:  make(chan *ExecutedArbitrage, 32),
	}
	s.dao = NewDao(url, scheme, user, passwd)
	return s
}

func (s *Store) Start() {
	go s.store()
}

func (s *Store) Stop() {

}

func (s *Store) store() {
	for {
		select {
		case arb := <-s.localChan:
			s.dao.SaveLocalArbitrage(arb)
		case arb := <-s.committedChan:
			s.dao.SaveCommittedArbitrage(arb)
		case arb := <-s.executedChan:
			s.dao.SaveExecutedArbitrage(arb)
		case <-s.ctx.Done():
			return
		}
	}
}

func (s *Store) StoreLocalArbitrage(arb *LocalArbitrage) {
	s.localChan <- arb
}

func (s *Store) StoreCommittedArbitrage(arb *CommittedArbitrage) {
	s.committedChan <- arb
}

func (s *Store) StoreExecutedArbitrage(arb *ExecutedArbitrage) {
	s.executedChan <- arb
}

func (s *Store) GetLocalArbitrage(id uint64) ([]*LocalArbitrage, error) {
	return s.dao.SelectLocalArbitrage(id)
}

func (s *Store) GetCommittedArbitrage(id uint64) ([]*CommittedArbitrage, error) {
	return s.dao.SelectCommittedArbitrage(id)
}

func (s *Store) GetExecutedArbitrage(id uint64) ([]*ExecutedArbitrage, error) {
	return s.dao.SelectExecutedArbitrage(id)
}
