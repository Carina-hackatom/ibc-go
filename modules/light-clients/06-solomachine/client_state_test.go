package solomachine_test

import (
	clienttypes "github.com/cosmos/ibc-go/v3/modules/core/02-client/types"
	commitmenttypes "github.com/cosmos/ibc-go/v3/modules/core/23-commitment/types"
	"github.com/cosmos/ibc-go/v3/modules/core/exported"
	solomachine "github.com/cosmos/ibc-go/v3/modules/light-clients/06-solomachine"
	ibctmtypes "github.com/cosmos/ibc-go/v3/modules/light-clients/07-tendermint"
	ibctesting "github.com/cosmos/ibc-go/v3/testing"
)

const (
	counterpartyClientIdentifier = "chainA"
	testConnectionID             = "connectionid"
	testChannelID                = "testchannelid"
	testPortID                   = "testportid"
)

func (suite *SoloMachineTestSuite) TestStatus() {
	clientState := suite.solomachine.ClientState()
	// solo machine discards arguments
	status := clientState.Status(suite.chainA.GetContext(), nil, nil)
	suite.Require().Equal(exported.Active, status)

	// freeze solo machine
	clientState.IsFrozen = true
	status = clientState.Status(suite.chainA.GetContext(), nil, nil)
	suite.Require().Equal(exported.Frozen, status)
}

func (suite *SoloMachineTestSuite) TestClientStateValidateBasic() {
	// test singlesig and multisig public keys
	for _, sm := range []*ibctesting.Solomachine{suite.solomachine, suite.solomachineMulti} {

		testCases := []struct {
			name        string
			clientState *solomachine.ClientState
			expPass     bool
		}{
			{
				"valid client state",
				sm.ClientState(),
				true,
			},
			{
				"empty ClientState",
				&solomachine.ClientState{},
				false,
			},
			{
				"sequence is zero",
				solomachine.NewClientState(0, &solomachine.ConsensusState{sm.ConsensusState().PublicKey, sm.Diversifier, sm.Time}, false),
				false,
			},
			{
				"timestamp is zero",
				solomachine.NewClientState(1, &solomachine.ConsensusState{sm.ConsensusState().PublicKey, sm.Diversifier, 0}, false),
				false,
			},
			{
				"diversifier is blank",
				solomachine.NewClientState(1, &solomachine.ConsensusState{sm.ConsensusState().PublicKey, "  ", 1}, false),
				false,
			},
			{
				"pubkey is empty",
				solomachine.NewClientState(1, &solomachine.ConsensusState{nil, sm.Diversifier, sm.Time}, false),
				false,
			},
		}

		for _, tc := range testCases {
			tc := tc

			suite.Run(tc.name, func() {

				err := tc.clientState.Validate()

				if tc.expPass {
					suite.Require().NoError(err)
				} else {
					suite.Require().Error(err)
				}
			})
		}
	}
}

func (suite *SoloMachineTestSuite) TestInitialize() {
	// test singlesig and multisig public keys
	for _, sm := range []*ibctesting.Solomachine{suite.solomachine, suite.solomachineMulti} {
		malleatedConsensus := sm.ClientState().ConsensusState
		malleatedConsensus.Timestamp = malleatedConsensus.Timestamp + 10

		testCases := []struct {
			name      string
			consState exported.ConsensusState
			expPass   bool
		}{
			{
				"valid consensus state",
				sm.ConsensusState(),
				true,
			},
			{
				"nil consensus state",
				nil,
				false,
			},
			{
				"invalid consensus state: Tendermint consensus state",
				&ibctmtypes.ConsensusState{},
				false,
			},
			{
				"invalid consensus state: consensus state does not match consensus state in client",
				malleatedConsensus,
				false,
			},
		}

		for _, tc := range testCases {
			err := sm.ClientState().Initialize(
				suite.chainA.GetContext(), suite.chainA.Codec,
				suite.chainA.App.GetIBCKeeper().ClientKeeper.ClientStore(suite.chainA.GetContext(), "solomachine"),
				tc.consState,
			)

			if tc.expPass {
				suite.Require().NoError(err, "valid testcase: %s failed", tc.name)
			} else {
				suite.Require().Error(err, "invalid testcase: %s passed", tc.name)
			}
		}
	}
}

func (suite *SoloMachineTestSuite) TestVerifyMembership() {
	// test singlesig and multisig public keys
	for _, sm := range []*ibctesting.Solomachine{suite.solomachine, suite.solomachineMulti} {

		var (
			clientState *solomachine.ClientState
			err         error
			height      clienttypes.Height
			path        []byte
			proof       []byte
			signBytes   solomachine.SignBytesV2
		)

		testCases := []struct {
			name     string
			malleate func()
			expPass  bool
		}{
			{
				"success",
				func() {},
				true,
			},
			{
				"success: client state verification",
				func() {
					merklePath := suite.solomachine.GetClientStatePath(counterpartyClientIdentifier)
					signBytes = solomachine.SignBytesV2{
						Sequence:    sm.GetHeight().GetRevisionHeight(),
						Timestamp:   sm.Time,
						Diversifier: sm.Diversifier,
						Path:        []byte(merklePath.String()),
						Data:        []byte("solomachine.ClientState"),
					}

					signBz, err := suite.chainA.Codec.Marshal(&signBytes)
					suite.Require().NoError(err)

					sig := sm.GenerateSignature(signBz)

					signatureDoc := &solomachine.TimestampedSignatureData{
						SignatureData: sig,
						Timestamp:     sm.Time,
					}

					path, err = suite.chainA.Codec.Marshal(&merklePath)
					suite.Require().NoError(err)

					proof, err = suite.chainA.Codec.Marshal(signatureDoc)
					suite.Require().NoError(err)

				},
				true,
			},
			{
				"success: consensus state verification",
				func() {
					merklePath := sm.GetConsensusStatePath(counterpartyClientIdentifier, height)
					signBytes = solomachine.SignBytesV2{
						Sequence:    sm.Sequence,
						Timestamp:   sm.Time,
						Diversifier: sm.Diversifier,
						Path:        []byte(merklePath.String()),
						Data:        []byte("solomachine.ConsensusState"),
					}

					signBz, err := suite.chainA.Codec.Marshal(&signBytes)
					suite.Require().NoError(err)

					sig := sm.GenerateSignature(signBz)

					signatureDoc := &solomachine.TimestampedSignatureData{
						SignatureData: sig,
						Timestamp:     sm.Time,
					}

					path, err = suite.chainA.Codec.Marshal(&merklePath)
					suite.Require().NoError(err)

					proof, err = suite.chainA.Codec.Marshal(signatureDoc)
					suite.Require().NoError(err)
				},
				true,
			},
			{
				"success: connection state verification",
				func() {
					merklePath := sm.GetConnectionStatePath(ibctesting.FirstConnectionID)
					signBytes = solomachine.SignBytesV2{
						Sequence:    sm.Sequence,
						Timestamp:   sm.Time,
						Diversifier: sm.Diversifier,
						Path:        []byte(merklePath.String()),
						Data:        []byte("solomachine.ConnectionState"),
					}

					signBz, err := suite.chainA.Codec.Marshal(&signBytes)
					suite.Require().NoError(err)

					sig := sm.GenerateSignature(signBz)

					signatureDoc := &solomachine.TimestampedSignatureData{
						SignatureData: sig,
						Timestamp:     sm.Time,
					}

					path, err = suite.chainA.Codec.Marshal(&merklePath)
					suite.Require().NoError(err)

					proof, err = suite.chainA.Codec.Marshal(signatureDoc)
					suite.Require().NoError(err)
				},
				true,
			},
			{
				"success: channel state verification",
				func() {
					merklePath := sm.GetChannelStatePath(ibctesting.MockPort, ibctesting.FirstChannelID)
					signBytes = solomachine.SignBytesV2{
						Sequence:    sm.Sequence,
						Timestamp:   sm.Time,
						Diversifier: sm.Diversifier,
						Path:        []byte(merklePath.String()),
						Data:        []byte("solomachine.ChannelState"),
					}

					signBz, err := suite.chainA.Codec.Marshal(&signBytes)
					suite.Require().NoError(err)

					sig := sm.GenerateSignature(signBz)

					signatureDoc := &solomachine.TimestampedSignatureData{
						SignatureData: sig,
						Timestamp:     sm.Time,
					}

					path, err = suite.chainA.Codec.Marshal(&merklePath)
					suite.Require().NoError(err)

					proof, err = suite.chainA.Codec.Marshal(signatureDoc)
					suite.Require().NoError(err)
				},
				true,
			},
			{
				"success: next sequence recv verification",
				func() {
					merklePath := sm.GetNextSequenceRecvPath(ibctesting.MockPort, ibctesting.FirstChannelID)
					signBytes = solomachine.SignBytesV2{
						Sequence:    sm.Sequence,
						Timestamp:   sm.Time,
						Diversifier: sm.Diversifier,
						Path:        []byte(merklePath.String()),
						Data:        []byte("solomachine.NextSequenceRecv"),
					}

					signBz, err := suite.chainA.Codec.Marshal(&signBytes)
					suite.Require().NoError(err)

					sig := sm.GenerateSignature(signBz)

					signatureDoc := &solomachine.TimestampedSignatureData{
						SignatureData: sig,
						Timestamp:     sm.Time,
					}

					path, err = suite.chainA.Codec.Marshal(&merklePath)
					suite.Require().NoError(err)

					proof, err = suite.chainA.Codec.Marshal(signatureDoc)
					suite.Require().NoError(err)
				},
				true,
			},
			{
				"success: packet commitment verification",
				func() {
					merklePath := sm.GetPacketCommitmentPath(ibctesting.MockPort, ibctesting.FirstChannelID)
					signBytes = solomachine.SignBytesV2{
						Sequence:    sm.Sequence,
						Timestamp:   sm.Time,
						Diversifier: sm.Diversifier,
						Path:        []byte(merklePath.String()),
						Data:        []byte("solomachine.PacketCommitment"),
					}

					signBz, err := suite.chainA.Codec.Marshal(&signBytes)
					suite.Require().NoError(err)

					sig := sm.GenerateSignature(signBz)

					signatureDoc := &solomachine.TimestampedSignatureData{
						SignatureData: sig,
						Timestamp:     sm.Time,
					}

					path, err = suite.chainA.Codec.Marshal(&merklePath)
					suite.Require().NoError(err)

					proof, err = suite.chainA.Codec.Marshal(signatureDoc)
					suite.Require().NoError(err)
				},
				true,
			},
			{
				"success: packet acknowledgement verification",
				func() {
					merklePath := sm.GetPacketAcknowledgementPath(ibctesting.MockPort, ibctesting.FirstChannelID)
					signBytes = solomachine.SignBytesV2{
						Sequence:    sm.Sequence,
						Timestamp:   sm.Time,
						Diversifier: sm.Diversifier,
						Path:        []byte(merklePath.String()),
						Data:        []byte("solomachine.PacketAcknowledgement"),
					}

					signBz, err := suite.chainA.Codec.Marshal(&signBytes)
					suite.Require().NoError(err)

					sig := sm.GenerateSignature(signBz)

					signatureDoc := &solomachine.TimestampedSignatureData{
						SignatureData: sig,
						Timestamp:     sm.Time,
					}

					path, err = suite.chainA.Codec.Marshal(&merklePath)
					suite.Require().NoError(err)

					proof, err = suite.chainA.Codec.Marshal(signatureDoc)
					suite.Require().NoError(err)
				},
				true,
			},
			{
				"success: packet receipt verification",
				func() {
					merklePath := sm.GetPacketReceiptPath(ibctesting.MockPort, ibctesting.FirstChannelID)
					signBytes = solomachine.SignBytesV2{
						Sequence:    sm.Sequence,
						Timestamp:   sm.Time,
						Diversifier: sm.Diversifier,
						Path:        []byte(merklePath.String()),
						Data:        []byte("solomachine.PacketReceipt"),
					}

					signBz, err := suite.chainA.Codec.Marshal(&signBytes)
					suite.Require().NoError(err)

					sig := sm.GenerateSignature(signBz)

					signatureDoc := &solomachine.TimestampedSignatureData{
						SignatureData: sig,
						Timestamp:     sm.Time,
					}

					path, err = suite.chainA.Codec.Marshal(&merklePath)
					suite.Require().NoError(err)

					proof, err = suite.chainA.Codec.Marshal(signatureDoc)
					suite.Require().NoError(err)
				},
				true,
			},
			{
				"consensus state in client state is nil",
				func() {
					clientState = solomachine.NewClientState(1, nil, false)
				},
				false,
			},
			{
				"client state latest height is less than sequence",
				func() {
					consensusState := &solomachine.ConsensusState{
						Timestamp: sm.Time,
						PublicKey: sm.ConsensusState().PublicKey,
					}

					clientState = solomachine.NewClientState(sm.Sequence-1, consensusState, false)
				},
				false,
			},
			{
				"height revision number is not zero",
				func() {
					height = clienttypes.NewHeight(1, sm.GetHeight().GetRevisionHeight())
				},
				false,
			},
			{
				"malformed merkle path fails to unmarshal",
				func() {
					path = []byte("invalid path")
				},
				false,
			},
			{
				"malformed proof fails to unmarshal",
				func() {
					merklePath := suite.solomachine.GetClientStatePath(counterpartyClientIdentifier)
					path, err = suite.chainA.Codec.Marshal(&merklePath)
					suite.Require().NoError(err)

					proof = []byte("invalid proof")
				},
				false,
			},
			{
				"consensus state timestamp is greater than signature",
				func() {
					consensusState := &solomachine.ConsensusState{
						Timestamp: sm.Time + 1,
						PublicKey: sm.ConsensusState().PublicKey,
					}

					clientState = solomachine.NewClientState(sm.Sequence, consensusState, false)
				},
				false,
			},
			{
				"signature data is nil",
				func() {
					signatureDoc := &solomachine.TimestampedSignatureData{
						SignatureData: nil,
						Timestamp:     sm.Time,
					}

					proof, err = suite.chainA.Codec.Marshal(signatureDoc)
					suite.Require().NoError(err)
				},
				false,
			},
			{
				"consensus state public key is nil",
				func() {
					clientState.ConsensusState.PublicKey = nil
				},
				false,
			},
			{
				"malformed signature data fails to unmarshal",
				func() {
					signatureDoc := &solomachine.TimestampedSignatureData{
						SignatureData: []byte("invalid signature data"),
						Timestamp:     sm.Time,
					}

					proof, err = suite.chainA.Codec.Marshal(signatureDoc)
					suite.Require().NoError(err)
				},
				false,
			},
			{
				"proof verification failed",
				func() {
					signBytes.Data = []byte("invalid membership data value")
				},
				false,
			},
		}

		for _, tc := range testCases {
			tc := tc

			suite.Run(tc.name, func() {
				clientState = sm.ClientState()
				height = clienttypes.NewHeight(sm.GetHeight().GetRevisionNumber(), sm.GetHeight().GetRevisionHeight())

				merklePath := commitmenttypes.NewMerklePath("ibc", "solomachine")
				signBytes = solomachine.SignBytesV2{
					Sequence:    sm.GetHeight().GetRevisionHeight(),
					Timestamp:   sm.Time,
					Diversifier: sm.Diversifier,
					Path:        []byte(merklePath.String()),
					Data:        []byte("solomachine"),
				}

				signBz, err := suite.chainA.Codec.Marshal(&signBytes)
				suite.Require().NoError(err)

				sig := sm.GenerateSignature(signBz)

				signatureDoc := &solomachine.TimestampedSignatureData{
					SignatureData: sig,
					Timestamp:     sm.Time,
				}

				path, err = suite.chainA.Codec.Marshal(&merklePath)
				suite.Require().NoError(err)

				proof, err = suite.chainA.Codec.Marshal(signatureDoc)
				suite.Require().NoError(err)

				tc.malleate()

				var expSeq uint64
				if clientState.ConsensusState != nil {
					expSeq = clientState.Sequence + 1
				}

				err = clientState.VerifyMembership(
					suite.chainA.GetContext(), suite.store, suite.chainA.Codec,
					height, 0, 0, // solomachine does not check delay periods
					proof, path, signBytes.Data,
				)

				if tc.expPass {
					suite.Require().NoError(err)
					suite.Require().Equal(expSeq, clientState.Sequence)
					suite.Require().Equal(expSeq, suite.GetSequenceFromStore(), "sequence not updated in the store (%d) on valid test case %s", suite.GetSequenceFromStore(), tc.name)
				} else {
					suite.Require().Error(err)
				}
			})
		}
	}
}

func (suite *SoloMachineTestSuite) TestGetTimestampAtHeight() {
	tmPath := ibctesting.NewPath(suite.chainA, suite.chainB)
	suite.coordinator.SetupClients(tmPath)
	// Single setup for all test cases.
	suite.SetupTest()

	testCases := []struct {
		name        string
		clientState *solomachine.ClientState
		height      exported.Height
		expValue    uint64
		expPass     bool
	}{
		{
			name:        "get timestamp at height exists",
			clientState: suite.solomachine.ClientState(),
			height:      suite.solomachine.ClientState().GetLatestHeight(),
			expValue:    suite.solomachine.ClientState().ConsensusState.Timestamp,
			expPass:     true,
		},
		{
			name:        "get timestamp at height not exists",
			clientState: suite.solomachine.ClientState(),
			height:      suite.solomachine.ClientState().GetLatestHeight().Increment(),
		},
	}

	for i, tc := range testCases {
		tc := tc

		suite.Run(tc.name, func() {
			ctx := suite.chainA.GetContext()

			ts, err := tc.clientState.GetTimestampAtHeight(
				ctx, suite.store, suite.chainA.Codec, tc.height,
			)

			suite.Require().Equal(tc.expValue, ts)

			if tc.expPass {
				suite.Require().NoError(err, "valid test case %d failed: %s", i, tc.name)
			} else {
				suite.Require().Error(err, "invalid test case %d passed: %s", i, tc.name)
			}
		})
	}
}