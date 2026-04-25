package kinds

import "testing"

func TestKindStringsAreNamespaced(t *testing.T) {
	cases := []struct {
		kind Kinder
		want string
	}{
		{ValidatorHeightAdvanced{}, "validator.height_advanced"},
		{ValidatorPeerConnected{}, "validator.peer_connected"},
		{ValidatorPeerDisconnected{}, "validator.peer_disconnected"},
		{ValidatorConsensusRoundStep{}, "validator.consensus.round_step"},
		{ValidatorVoteCast{}, "validator.vote_cast"},
		{ValidatorVoteMissed{}, "validator.vote_missed"},
		{ValidatorProposed{}, "validator.proposed"},
		{ValidatorSignedBlock{}, "validator.signed_block"},
		{ValidatorConfigChanged{}, "validator.config_changed"},
		{ValidatorWentOffline{}, "validator.went_offline"},
		{ValidatorCameOnline{}, "validator.came_online"},
		{ChainBlockCommitted{}, "chain.block_committed"},
		{ChainValsetChanged{}, "chain.valset_changed"},
		{ChainTxExecuted{}, "chain.tx_executed"},
		{ChainConsensusStuck{}, "chain.consensus_stuck"},
	}
	for _, c := range cases {
		if c.kind.Kind() != c.want {
			t.Errorf("Kind() = %q, want %q", c.kind.Kind(), c.want)
		}
	}
}

func TestAllKindsRegistered(t *testing.T) {
	if len(All()) < 15 {
		t.Errorf("expected ≥15 registered kinds, got %d", len(All()))
	}
}
