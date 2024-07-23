/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package checkpoint

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/kubernetes/pkg/kubelet/checkpointmanager"
	testutil "k8s.io/kubernetes/pkg/kubelet/cm/cpumanager/state/testing"
	state "k8s.io/kubernetes/pkg/kubelet/cm/dra/state/v1"
)

const testingCheckpoint = "dramanager_checkpoint_test"

// TODO (https://github.com/kubernetes/kubernetes/issues/123552): reconsider what data gets stored in checkpoints and whether that is really necessary.
//
// As it stands now, a "v1" checkpoint contains data for types like the resourceapi.ResourceHandle
// which may change over time as new fields get added in a backward-compatible way (not unusual
// for API types). That breaks checksuming with pkg/util/hash because it is based on spew output.
// That output includes those new fields.

func TestCheckpointGetOrCreate(t *testing.T) {
	testCases := []struct {
		description                string
		checkpointContent          string
		expectedError              string
		expectedClaimInfoStateList state.ClaimInfoStateList
	}{
		{
			description:                "new-checkpoint",
			expectedClaimInfoStateList: state.ClaimInfoStateList{},
		},
		{
			description:       "single-claim-info-state",
			checkpointContent: `{"Data":"{\"kind\":\"DRACheckpoint\",\"apiVersion\":\"checkpoint.dra.kubelet.k8s.io/v1\",\"Entries\":[{\"ClaimUID\":\"067798be-454e-4be4-9047-1aa06aea63f7\",\"ClaimName\":\"example\",\"Namespace\":\"default\",\"PodUIDs\":{\"139cdb46-f989-4f17-9561-ca10cfb509a6\":{}},\"DriverState\":{\"test-driver.cdi.k8s.io\":{\"Devices\":[{\"PoolName\":\"worker-1\",\"DeviceName\":\"dev-1\",\"RequestNames\":[\"test request\"],\"CDIDeviceIDs\":[\"example.com/example=cdi-example\"]}]}}}]}","Checksum":2327868095}`,
			expectedClaimInfoStateList: state.ClaimInfoStateList{
				{
					DriverState: map[string]state.DriverState{
						"test-driver.cdi.k8s.io": {
							Devices: []state.Device{
								{
									PoolName:     "worker-1",
									DeviceName:   "dev-1",
									RequestNames: []string{"test request"},
									CDIDeviceIDs: []string{"example.com/example=cdi-example"},
								},
							},
						},
					},
					ClaimUID:  "067798be-454e-4be4-9047-1aa06aea63f7",
					ClaimName: "example",
					Namespace: "default",
					PodUIDs:   sets.New("139cdb46-f989-4f17-9561-ca10cfb509a6"),
				},
			},
		},
		{
			description:       "claim-info-state-with-multiple-devices",
			checkpointContent: `{"Data":"{\"kind\":\"DRACheckpoint\",\"apiVersion\":\"checkpoint.dra.kubelet.k8s.io/v1\",\"Entries\":[{\"ClaimUID\":\"067798be-454e-4be4-9047-1aa06aea63f7\",\"ClaimName\":\"example\",\"Namespace\":\"default\",\"PodUIDs\":{\"139cdb46-f989-4f17-9561-ca10cfb509a6\":{}},\"DriverState\":{\"test-driver.cdi.k8s.io\":{\"Devices\":[{\"PoolName\":\"worker-1\",\"DeviceName\":\"dev-1\",\"RequestNames\":[\"test request\"],\"CDIDeviceIDs\":[\"example.com/example=cdi-example\"]},{\"PoolName\":\"worker-1\",\"DeviceName\":\"dev-2\",\"RequestNames\":[\"test request\"],\"CDIDeviceIDs\":[\"example.com/example=cdi-example\"]}]}}}]}","Checksum":1228427175}`,
			expectedClaimInfoStateList: state.ClaimInfoStateList{
				{
					DriverState: map[string]state.DriverState{
						"test-driver.cdi.k8s.io": {
							Devices: []state.Device{
								{
									PoolName:     "worker-1",
									DeviceName:   "dev-1",
									RequestNames: []string{"test request"},
									CDIDeviceIDs: []string{"example.com/example=cdi-example"},
								},
								{
									PoolName:     "worker-1",
									DeviceName:   "dev-2",
									RequestNames: []string{"test request"},
									CDIDeviceIDs: []string{"example.com/example=cdi-example"},
								},
							},
						},
					},
					ClaimUID:  "067798be-454e-4be4-9047-1aa06aea63f7",
					ClaimName: "example",
					Namespace: "default",
					PodUIDs:   sets.New("139cdb46-f989-4f17-9561-ca10cfb509a6"),
				},
			},
		},
		{
			description:       "two-claim-info-states",
			checkpointContent: `{"Data":"{\"kind\":\"DRACheckpoint\",\"apiVersion\":\"checkpoint.dra.kubelet.k8s.io/v1\",\"Entries\":[{\"ClaimUID\":\"067798be-454e-4be4-9047-1aa06aea63f7\",\"ClaimName\":\"example-1\",\"Namespace\":\"default\",\"PodUIDs\":{\"139cdb46-f989-4f17-9561-ca10cfb509a6\":{}},\"DriverState\":{\"test-driver.cdi.k8s.io\":{\"Devices\":[{\"PoolName\":\"worker-1\",\"DeviceName\":\"dev-1\",\"RequestNames\":[\"test request\"],\"CDIDeviceIDs\":[\"example.com/example=cdi-example\"]}]}}},{\"ClaimUID\":\"4cf8db2d-06c0-7d70-1a51-e59b25b2c16c\",\"ClaimName\":\"example-2\",\"Namespace\":\"default\",\"PodUIDs\":{\"139cdb46-f989-4f17-9561-ca10cfb509a6\":{}},\"DriverState\":{\"test-driver.cdi.k8s.io\":{\"Devices\":[{\"PoolName\":\"worker-1\",\"DeviceName\":\"dev-2\",\"RequestNames\":null,\"CDIDeviceIDs\":null}]}}}]}","Checksum":2930258364}`,
			expectedClaimInfoStateList: state.ClaimInfoStateList{
				{
					DriverState: map[string]state.DriverState{
						"test-driver.cdi.k8s.io": {
							Devices: []state.Device{
								{
									PoolName:     "worker-1",
									DeviceName:   "dev-1",
									RequestNames: []string{"test request"},
									CDIDeviceIDs: []string{"example.com/example=cdi-example"},
								},
							},
						},
					},
					ClaimUID:  "067798be-454e-4be4-9047-1aa06aea63f7",
					ClaimName: "example-1",
					Namespace: "default",
					PodUIDs:   sets.New("139cdb46-f989-4f17-9561-ca10cfb509a6"),
				},
				{
					DriverState: map[string]state.DriverState{
						"test-driver.cdi.k8s.io": {
							Devices: []state.Device{
								{
									PoolName:   "worker-1",
									DeviceName: "dev-2",
								},
							},
						},
					},
					ClaimUID:  "4cf8db2d-06c0-7d70-1a51-e59b25b2c16c",
					ClaimName: "example-2",
					Namespace: "default",
					PodUIDs:   sets.New("139cdb46-f989-4f17-9561-ca10cfb509a6"),
				},
			},
		},
		{
			description:       "incorrect-checksum",
			checkpointContent: `{"Data":"{\"kind\":\"DRACheckpoint\",\"apiVersion\":\"checkpoint.dra.kubelet.k8s.io/v1\",\"Entries\":[{\"ClaimUID\":\"067798be-454e-4be4-9047-1aa06aea63f7\",\"ClaimName\":\"example-1\",\"Namespace\":\"default\",\"PodUIDs\":{\"139cdb46-f989-4f17-9561-ca10cfb509a6\":{}},\"DriverState\":{\"test-driver.cdi.k8s.io\":{\"Devices\":[{\"PoolName\":\"worker-1\",\"DeviceName\":\"dev-1\",\"RequestNames\":[\"test request\"],\"CDIDeviceIDs\":[\"example.com/example=cdi-example\"]}]}}},{\"ClaimUID\":\"4cf8db2d-06c0-7d70-1a51-e59b25b2c16c\",\"ClaimName\":\"example-2\",\"Namespace\":\"default\",\"PodUIDs\":{\"139cdb46-f989-4f17-9561-ca10cfb509a6\":{}},\"DriverState\":{\"test-driver.cdi.k8s.io\":{\"Devices\":[{\"PoolName\":\"worker-1\",\"DeviceName\":\"dev-2\",\"RequestNames\":null,\"CDIDeviceIDs\":null}]}}}]}","Checksum":2930258365}`,
			expectedError:     "checkpoint is corrupted",
		},
		{
			description:       "invalid-JSON",
			checkpointContent: `{`,
			expectedError:     "unexpected end of JSON input",
		},
	}

	// create temp dir
	testingDir, err := os.MkdirTemp("", "dramanager_state_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(testingDir)

	// create checkpoint manager for testing
	cpm, err := checkpointmanager.NewCheckpointManager(testingDir)
	assert.NoError(t, err, "could not create testing checkpoint manager")

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			// ensure there is no previous checkpoint
			assert.NoError(t, cpm.RemoveCheckpoint(testingCheckpoint), "could not remove testing checkpoint")

			// prepare checkpoint for testing
			if strings.TrimSpace(tc.checkpointContent) != "" {
				mock := &testutil.MockCheckpoint{Content: tc.checkpointContent}
				assert.NoError(t, cpm.CreateCheckpoint(testingCheckpoint, mock), "could not create testing checkpoint")
			}

			checkpointer, err := NewCheckpointer(testingDir, testingCheckpoint)
			assert.NoError(t, err, "could not create testing checkpointer")

			checkpoint, err := checkpointer.GetOrCreate()
			if strings.TrimSpace(tc.expectedError) != "" {
				assert.ErrorContains(t, err, tc.expectedError)
			} else {
				assert.NoError(t, err, "unexpected error")
				stateList, err := checkpoint.GetEntries()
				assert.NoError(t, err, "could not get data entries from checkpoint")
				require.NoError(t, err)
				assert.Equal(t, tc.expectedClaimInfoStateList, stateList)
			}
		})
	}
}

func TestCheckpointStateStore(t *testing.T) {
	testCases := []struct {
		description               string
		claimInfoStateList        state.ClaimInfoStateList
		expectedCheckpointContent string
	}{
		{
			description: "single-claim-info-state",
			claimInfoStateList: state.ClaimInfoStateList{
				{
					DriverState: map[string]state.DriverState{
						"test-driver.cdi.k8s.io": {
							Devices: []state.Device{
								{
									PoolName:     "worker-1",
									DeviceName:   "dev-1",
									RequestNames: []string{"test request"},
									CDIDeviceIDs: []string{"example.com/example=cdi-example"},
								},
							},
						},
					},
					ClaimUID:  "067798be-454e-4be4-9047-1aa06aea63f7",
					ClaimName: "example",
					Namespace: "default",
					PodUIDs:   sets.New("139cdb46-f989-4f17-9561-ca10cfb509a6"),
				},
			},
			expectedCheckpointContent: `{"Data":"{\"kind\":\"DRACheckpoint\",\"apiVersion\":\"checkpoint.dra.kubelet.k8s.io/v1\",\"Entries\":[{\"ClaimUID\":\"067798be-454e-4be4-9047-1aa06aea63f7\",\"ClaimName\":\"example\",\"Namespace\":\"default\",\"PodUIDs\":{\"139cdb46-f989-4f17-9561-ca10cfb509a6\":{}},\"DriverState\":{\"test-driver.cdi.k8s.io\":{\"Devices\":[{\"PoolName\":\"worker-1\",\"DeviceName\":\"dev-1\",\"RequestNames\":[\"test request\"],\"CDIDeviceIDs\":[\"example.com/example=cdi-example\"]}]}}}]}","Checksum":2327868095}`,
		},
		{
			description: "single-claim-info-state-with-multiple-devices",
			claimInfoStateList: state.ClaimInfoStateList{
				{
					DriverState: map[string]state.DriverState{
						"test-driver.cdi.k8s.io": {
							Devices: []state.Device{
								{
									PoolName:     "worker-1",
									DeviceName:   "dev-1",
									RequestNames: []string{"test request"},
									CDIDeviceIDs: []string{"example.com/example=cdi-example"},
								},
								{
									PoolName:     "worker-1",
									DeviceName:   "dev-2",
									RequestNames: []string{"test request"},
									CDIDeviceIDs: []string{"example.com/example=cdi-example"},
								},
							},
						},
					},
					ClaimUID:  "067798be-454e-4be4-9047-1aa06aea63f7",
					ClaimName: "example",
					Namespace: "default",
					PodUIDs:   sets.New("139cdb46-f989-4f17-9561-ca10cfb509a6"),
				},
			},
			expectedCheckpointContent: `{"Data":"{\"kind\":\"DRACheckpoint\",\"apiVersion\":\"checkpoint.dra.kubelet.k8s.io/v1\",\"Entries\":[{\"ClaimUID\":\"067798be-454e-4be4-9047-1aa06aea63f7\",\"ClaimName\":\"example\",\"Namespace\":\"default\",\"PodUIDs\":{\"139cdb46-f989-4f17-9561-ca10cfb509a6\":{}},\"DriverState\":{\"test-driver.cdi.k8s.io\":{\"Devices\":[{\"PoolName\":\"worker-1\",\"DeviceName\":\"dev-1\",\"RequestNames\":[\"test request\"],\"CDIDeviceIDs\":[\"example.com/example=cdi-example\"]},{\"PoolName\":\"worker-1\",\"DeviceName\":\"dev-2\",\"RequestNames\":[\"test request\"],\"CDIDeviceIDs\":[\"example.com/example=cdi-example\"]}]}}}]}","Checksum":1228427175}`,
		},
		{
			description: "two-claim-info-states",
			claimInfoStateList: state.ClaimInfoStateList{
				{
					DriverState: map[string]state.DriverState{
						"test-driver.cdi.k8s.io": {
							Devices: []state.Device{
								{
									PoolName:     "worker-1",
									DeviceName:   "dev-1",
									RequestNames: []string{"test request"},
									CDIDeviceIDs: []string{"example.com/example=cdi-example"},
								},
							},
						},
					},
					ClaimUID:  "067798be-454e-4be4-9047-1aa06aea63f7",
					ClaimName: "example-1",
					Namespace: "default",
					PodUIDs:   sets.New("139cdb46-f989-4f17-9561-ca10cfb509a6"),
				},
				{
					DriverState: map[string]state.DriverState{
						"test-driver.cdi.k8s.io": {
							Devices: []state.Device{
								{
									PoolName:   "worker-1",
									DeviceName: "dev-2",
								},
							},
						},
					},
					ClaimUID:  "4cf8db2d-06c0-7d70-1a51-e59b25b2c16c",
					ClaimName: "example-2",
					Namespace: "default",
					PodUIDs:   sets.New("139cdb46-f989-4f17-9561-ca10cfb509a6"),
				},
			},
			expectedCheckpointContent: `{"Data":"{\"kind\":\"DRACheckpoint\",\"apiVersion\":\"checkpoint.dra.kubelet.k8s.io/v1\",\"Entries\":[{\"ClaimUID\":\"067798be-454e-4be4-9047-1aa06aea63f7\",\"ClaimName\":\"example-1\",\"Namespace\":\"default\",\"PodUIDs\":{\"139cdb46-f989-4f17-9561-ca10cfb509a6\":{}},\"DriverState\":{\"test-driver.cdi.k8s.io\":{\"Devices\":[{\"PoolName\":\"worker-1\",\"DeviceName\":\"dev-1\",\"RequestNames\":[\"test request\"],\"CDIDeviceIDs\":[\"example.com/example=cdi-example\"]}]}}},{\"ClaimUID\":\"4cf8db2d-06c0-7d70-1a51-e59b25b2c16c\",\"ClaimName\":\"example-2\",\"Namespace\":\"default\",\"PodUIDs\":{\"139cdb46-f989-4f17-9561-ca10cfb509a6\":{}},\"DriverState\":{\"test-driver.cdi.k8s.io\":{\"Devices\":[{\"PoolName\":\"worker-1\",\"DeviceName\":\"dev-2\",\"RequestNames\":null,\"CDIDeviceIDs\":null}]}}}]}","Checksum":2930258364}`,
		},
	}

	// Should return an error, stateDir cannot be an empty string
	if _, err := NewCheckpointer("", testingCheckpoint); err == nil {
		t.Fatal("expected error but got nil")
	}

	// create temp dir
	testingDir, err := os.MkdirTemp("", "dramanager_state_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(testingDir)

	// NewCheckpointState with an empty checkpointName should return an error
	if _, err = NewCheckpointer(testingDir, ""); err == nil {
		t.Fatal("expected error but got nil")
	}

	cpm, err := checkpointmanager.NewCheckpointManager(testingDir)
	assert.NoError(t, err, "could not create testing checkpoint manager")
	assert.NoError(t, cpm.RemoveCheckpoint(testingCheckpoint), "could not remove testing checkpoint")

	cs, err := NewCheckpointer(testingDir, testingCheckpoint)
	assert.NoError(t, err, "could not create testing checkpointState instance")

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			checkpoint, err := NewCheckpoint(tc.claimInfoStateList)
			assert.NoError(t, err, "could not create Checkpoint")

			err = cs.Store(checkpoint)
			assert.NoError(t, err, "could not store checkpoint")

			cpm.GetCheckpoint(testingCheckpoint, checkpoint)

			checkpointContent, err := checkpoint.MarshalCheckpoint()
			assert.NoError(t, err, "could not Marshal Checkpoint")
			assert.Equal(t, tc.expectedCheckpointContent, string(checkpointContent))
		})
	}
}
