package zonemerger

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/provenance"
	"github.com/memoria-space/meking/zone"
)

type mergeSourceIdentity struct {
	ChildZoneID   zone.ID
	ObjectType    string
	ChildID       string
	ChildVersion  knowledge.Version
	ParentID      string
	ParentVersion knowledge.Version
	ParentHash    knowledge.Hash
}

type conflictSourceIdentity struct {
	ChildZoneID   zone.ID
	ObjectType    string
	ChildID       string
	ChildBase     knowledge.Version
	ParentZoneID  zone.ID
	ParentID      string
	ParentVersion knowledge.Version
	CandidateHash knowledge.Hash
}

func childMergeSource(identity mergeSourceIdentity) (provenance.Source, error) {
	encoded, err := json.Marshal(identity)
	if err != nil {
		return provenance.Source{}, fmt.Errorf("encode Child merge source: %w", err)
	}
	digest := sourceDigest(encoded)
	return provenance.Source{
		ID:         "source/child-zone/merge/" + digest,
		Kind:       provenance.ChildZone,
		ProducerID: string(identity.ChildZoneID) + "/" + identity.ObjectType + "/" + identity.ChildID + "@" + fmt.Sprint(identity.ChildVersion),
	}, nil
}

func entityAssignmentSource(conflict EntityConflict) (provenance.Source, error) {
	return conflictSource(conflictSourceIdentity{
		ChildZoneID:   conflict.ChildZoneID,
		ObjectType:    "entity",
		ChildID:       string(conflict.ChildEntityID),
		ChildBase:     conflict.ChildBase,
		ParentZoneID:  conflict.ParentZoneID,
		ParentID:      string(conflict.ParentEntityID),
		ParentVersion: conflict.ParentVersion,
		CandidateHash: conflict.CandidateHash,
	})
}

func relationAssignmentSource(conflict RelationConflict) (provenance.Source, error) {
	return conflictSource(conflictSourceIdentity{
		ChildZoneID:   conflict.ChildZoneID,
		ObjectType:    "relation",
		ChildID:       string(conflict.ChildRelationID),
		ChildBase:     conflict.ChildBase,
		ParentZoneID:  conflict.ParentZoneID,
		ParentID:      string(conflict.ParentRelationID),
		ParentVersion: conflict.ParentVersion,
		CandidateHash: conflict.CandidateHash,
	})
}

func claimAssignmentSource(conflict ClaimConflict) (provenance.Source, error) {
	return conflictSource(conflictSourceIdentity{
		ChildZoneID:   conflict.ChildZoneID,
		ObjectType:    "claim",
		ChildID:       string(conflict.ChildClaimID),
		ChildBase:     conflict.ChildBase,
		ParentZoneID:  conflict.ParentZoneID,
		ParentID:      string(conflict.ParentClaimID),
		ParentVersion: conflict.ParentVersion,
		CandidateHash: conflict.CandidateHash,
	})
}

func conflictSource(identity conflictSourceIdentity) (provenance.Source, error) {
	encoded, err := json.Marshal(identity)
	if err != nil {
		return provenance.Source{}, fmt.Errorf("encode Child conflict source: %w", err)
	}
	digest := sourceDigest(encoded)
	return provenance.Source{
		ID:         "source/child-zone/conflict/" + digest,
		Kind:       provenance.ChildZone,
		ProducerID: string(identity.ParentZoneID) + "/" + identity.ObjectType + "/" + identity.ParentID + "@" + fmt.Sprint(identity.ParentVersion),
	}, nil
}

func parentSupportSource(child provenance.Source, assignmentDigest string) (provenance.Source, error) {
	if err := requireResolutionSource(child); err != nil {
		return provenance.Source{}, err
	}
	return provenance.Source{
		ID:         child.ID + "/parent-support/" + assignmentDigest,
		Kind:       provenance.ChildZone,
		ProducerID: child.ProducerID + "/parent-support/" + assignmentDigest,
	}, nil
}

func parentDecisionSource(child provenance.Source, assignmentDigest string) (provenance.Source, error) {
	if err := requireResolutionSource(child); err != nil {
		return provenance.Source{}, err
	}
	return provenance.Source{
		ID:         child.ID + "/parent-decision/" + assignmentDigest,
		Kind:       provenance.Resolution,
		ProducerID: child.ProducerID + "/parent-decision/" + assignmentDigest,
	}, nil
}

func requireResolutionSource(source provenance.Source) error {
	if err := provenance.Validate(source); err != nil {
		return err
	}
	if source.Kind != provenance.Resolution {
		return fmt.Errorf("%w: Child conflict decision requires a Resolution Source", provenance.ErrInvalidSource)
	}
	return nil
}

func entityAssignmentDigest(conflict EntityConflict) (string, error) {
	encoded, err := json.Marshal(conflict)
	if err != nil {
		return "", fmt.Errorf("encode Entity conflict assignment: %w", err)
	}
	return sourceDigest(encoded), nil
}

func relationAssignmentDigest(conflict RelationConflict) (string, error) {
	encoded, err := json.Marshal(conflict)
	if err != nil {
		return "", fmt.Errorf("encode Relation conflict assignment: %w", err)
	}
	return sourceDigest(encoded), nil
}

func claimAssignmentDigest(conflict ClaimConflict) (string, error) {
	encoded, err := json.Marshal(conflict)
	if err != nil {
		return "", fmt.Errorf("encode Claim conflict assignment: %w", err)
	}
	return sourceDigest(encoded), nil
}

func sourceDigest(encoded []byte) string {
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}
