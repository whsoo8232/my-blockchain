package operations

import (
	"context"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/bazelbuild/rules_go/go/tools/bazel"
	"github.com/golang/snappy"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/blocks"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	state_native "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/spectest/utils"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"google.golang.org/protobuf/proto"
	"gopkg.in/d4l3k/messagediff.v1"
)

// RunBlockHeaderTest executes "operations/block_header" tests.
func RunBlockHeaderTest(t *testing.T, config string) {
	require.NoError(t, utils.SetConfig(t, config))
	testFolders, testsFolderPath := utils.TestFolders(t, config, "phase0", "operations/block_header/pyspec_tests")
	if len(testFolders) == 0 {
		t.Fatalf("No test folders found for %s/%s/%s", config, "phase0", "operations/block_header/pyspec_tests")
	}

	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			helpers.ClearCache()

			blockFile, err := util.BazelFileBytes(testsFolderPath, folder.Name(), "block.ssz_snappy")
			require.NoError(t, err)
			blockSSZ, err := snappy.Decode(nil /* dst */, blockFile)
			require.NoError(t, err, "Failed to decompress")
			block := &ethpb.BeaconBlock{}
			require.NoError(t, block.UnmarshalSSZ(blockSSZ), "Failed to unmarshal")

			preBeaconStateFile, err := util.BazelFileBytes(testsFolderPath, folder.Name(), "pre.ssz_snappy")
			require.NoError(t, err)
			preBeaconStateSSZ, err := snappy.Decode(nil /* dst */, preBeaconStateFile)
			require.NoError(t, err, "Failed to decompress")
			preBeaconStateBase := &ethpb.BeaconState{}
			require.NoError(t, preBeaconStateBase.UnmarshalSSZ(preBeaconStateSSZ), "Failed to unmarshal")
			preBeaconState, err := state_native.InitializeFromProtoPhase0(preBeaconStateBase)
			require.NoError(t, err)

			// If the post.ssz is not present, it means the test should fail on our end.
			postSSZFilepath, err := bazel.Runfile(path.Join(testsFolderPath, folder.Name(), "post.ssz_snappy"))
			postSSZExists := true
			if err != nil && strings.Contains(err.Error(), "could not locate file") {
				postSSZExists = false
			} else {
				require.NoError(t, err)
			}

			// Spectest blocks are not signed, so we'll call NoVerify to skip sig verification.
			bodyRoot, err := block.Body.HashTreeRoot()
			require.NoError(t, err)
			beaconState, err := blocks.ProcessBlockHeaderNoVerify(context.Background(), preBeaconState, block.Slot, block.ProposerIndex, block.ParentRoot, bodyRoot[:])
			if postSSZExists {
				require.NoError(t, err)

				postBeaconStateFile, err := os.ReadFile(postSSZFilepath) // #nosec G304
				require.NoError(t, err)
				postBeaconStateSSZ, err := snappy.Decode(nil /* dst */, postBeaconStateFile)
				require.NoError(t, err, "Failed to decompress")

				postBeaconState := &ethpb.BeaconState{}
				require.NoError(t, postBeaconState.UnmarshalSSZ(postBeaconStateSSZ), "Failed to unmarshal")
				pbState, err := state_native.ProtobufBeaconStatePhase0(beaconState.ToProto())
				require.NoError(t, err)
				if !proto.Equal(pbState, postBeaconState) {
					diff, _ := messagediff.PrettyDiff(beaconState.ToProto(), postBeaconState)
					t.Log(diff)
					t.Fatal("Post state does not match expected")
				}
			} else {
				// Note: This doesn't test anything worthwhile. It essentially tests
				// that *any* error has occurred, not any specific error.
				if err == nil {
					t.Fatal("Did not fail when expected")
				}
				t.Logf("Expected failure; failure reason = %v", err)
				return
			}
		})
	}
}