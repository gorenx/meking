package sqlite

import (
	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/deletion"
	"github.com/memoria-space/meking/knowledge/extraction"
	"github.com/memoria-space/meking/knowledge/resolution"
	"github.com/memoria-space/meking/knowledge/submission"
	"github.com/memoria-space/meking/knowledge/zonemerger"
)

var (
	_ knowledge.ViewSource                                              = (*Database)(nil)
	_ submission.KnowledgeIdentities                                    = (*Database)(nil)
	_ knowledge.VersionHistory                                          = (*Database)(nil)
	_ knowledge.CurrentVersions                                         = (*Database)(nil)
	_ submission.Sources                                                = (*Database)(nil)
	_ submission.Candidates                                             = (*Candidates)(nil)
	_ resolution.KnowledgeIdentities                                    = (*Database)(nil)
	_ resolution.Candidates                                             = (*Candidates)(nil)
	_ resolution.Provenance                                             = (*Database)(nil)
	_ deletion.KnowledgeReferences                                      = (*Database)(nil)
	_ deletion.Provenance                                               = (*Database)(nil)
	_ extraction.Progress                                               = (*Database)(nil)
	_ extraction.Manifests                                              = (*Database)(nil)
	_ zonemerger.Conflicts                                              = (*Database)(nil)
	_ zonemerger.Identities                                             = (*Database)(nil)
	_ knowledge.IdentityReader                                          = identityReader{}
	_ knowledge.VersionReader[knowledge.Entity, knowledge.EntityID]     = entityReader{}
	_ knowledge.VersionReader[knowledge.Relation, knowledge.RelationID] = relationReader{}
	_ knowledge.VersionReader[knowledge.Claim, knowledge.ClaimID]       = claimReader{}
)
