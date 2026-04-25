// Package kinds defines the typed payload struct for every scribe event kind.
//
// Each kind is a Go struct implementing Kinder. Payload fields map directly to
// the JSON `payload` column on the events table. Field evolution is ADDITIVE
// ONLY — adding fields is fine; renaming or removing fields requires a new
// kind (e.g. _v2 suffix). This invariant is what makes the events table safe
// to scan across schema generations.
package kinds

import "time"

// Kinder is implemented by every event payload struct.
type Kinder interface {
	Kind() string
}

// ---- validator kinds

type ValidatorHeightAdvanced struct {
	From int64 `json:"from"`
	To   int64 `json:"to"`
}

func (ValidatorHeightAdvanced) Kind() string { return "validator.height_advanced" }

type ValidatorPeerConnected struct {
	Peer      string `json:"peer"`
	PeerID    string `json:"peer_id"`
	Direction string `json:"direction"` // "in" or "out"
}

func (ValidatorPeerConnected) Kind() string { return "validator.peer_connected" }

type ValidatorPeerDisconnected struct {
	Peer   string `json:"peer"`
	Reason string `json:"reason"`
}

func (ValidatorPeerDisconnected) Kind() string { return "validator.peer_disconnected" }

type ValidatorConsensusRoundStep struct {
	Height int64  `json:"height"`
	Round  int32  `json:"round"`
	Step   string `json:"step"` // Propose|Prevote|Precommit|Commit|...
}

func (ValidatorConsensusRoundStep) Kind() string { return "validator.consensus.round_step" }

type ValidatorVoteCast struct {
	Height     int64  `json:"height"`
	Round      int32  `json:"round"`
	VoteType   string `json:"vote_type"` // "prevote" | "precommit"
	TargetHash string `json:"target_hash,omitempty"`
}

func (ValidatorVoteCast) Kind() string { return "validator.vote_cast" }

type ValidatorVoteMissed struct {
	Height   int64  `json:"height"`
	Round    int32  `json:"round"`
	VoteType string `json:"vote_type"`
}

func (ValidatorVoteMissed) Kind() string { return "validator.vote_missed" }

type ValidatorProposed struct {
	Height int64 `json:"height"`
	Round  int32 `json:"round"`
}

func (ValidatorProposed) Kind() string { return "validator.proposed" }

type ValidatorSignedBlock struct {
	Height int64 `json:"height"`
}

func (ValidatorSignedBlock) Kind() string { return "validator.signed_block" }

type ValidatorConfigChanged struct {
	Key string `json:"key"`
	Old string `json:"old"`
	New string `json:"new"`
}

func (ValidatorConfigChanged) Kind() string { return "validator.config_changed" }

type ValidatorWentOffline struct {
	LastSeen time.Time `json:"last_seen"`
}

func (ValidatorWentOffline) Kind() string { return "validator.went_offline" }

type ValidatorCameOnline struct {
	GapDuration string `json:"gap_duration"` // "5m23s"
}

func (ValidatorCameOnline) Kind() string { return "validator.came_online" }

// ---- chain kinds

type ChainBlockCommitted struct {
	Height   int64     `json:"height"`
	Time     time.Time `json:"time"`
	Txs      int32     `json:"txs"`
	Proposer string    `json:"proposer"`
	Round    int32     `json:"round"`
	BuildMs  int64     `json:"build_ms"`
	Size     int64     `json:"size"`
}

func (ChainBlockCommitted) Kind() string { return "chain.block_committed" }

type ChainValsetChanged struct {
	Height  int64          `json:"height"`
	Added   []ValsetMember `json:"added"`
	Removed []ValsetMember `json:"removed"`
}

func (ChainValsetChanged) Kind() string { return "chain.valset_changed" }

type ValsetMember struct {
	Address     string `json:"address"`
	VotingPower int64  `json:"voting_power"`
}

type ChainTxExecuted struct {
	Height    int64  `json:"height"`
	Type      string `json:"type"` // "add_pkg" | "call" | "run" | "send"
	Success   bool   `json:"success"`
	GasWanted int64  `json:"gas_wanted"`
	GasUsed   int64  `json:"gas_used"`
	Error     string `json:"error,omitempty"`
}

func (ChainTxExecuted) Kind() string { return "chain.tx_executed" }

type ChainConsensusStuck struct {
	Height    int64 `json:"height"`
	LastRound int32 `json:"last_round"`
}

func (ChainConsensusStuck) Kind() string { return "chain.consensus_stuck" }

// All returns every registered Kinder, used by /api/event-kinds and tests.
func All() []Kinder {
	return []Kinder{
		ValidatorHeightAdvanced{},
		ValidatorPeerConnected{},
		ValidatorPeerDisconnected{},
		ValidatorConsensusRoundStep{},
		ValidatorVoteCast{},
		ValidatorVoteMissed{},
		ValidatorProposed{},
		ValidatorSignedBlock{},
		ValidatorConfigChanged{},
		ValidatorWentOffline{},
		ValidatorCameOnline{},
		ChainBlockCommitted{},
		ChainValsetChanged{},
		ChainTxExecuted{},
		ChainConsensusStuck{},
	}
}
