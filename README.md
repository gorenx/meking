# Meking

English | [简体中文](README.zh-CN.md)

**Local knowledge memory for AI agents, with activation driven by natural recall.**

Meking stores conversations and documents as versioned entities, relations, and claims, with links back to their source evidence. Agents can save and retrieve memory through MCP; a built-in Web UI lets you inspect the knowledge graph, manage documents, and ask questions.

Its memory activation model uses evidence from recall that occurs naturally in conversation to update memory stability and difficulty, and estimate activation over time. Each accepted observation is retained so state changes can be replayed and verified.

Written in Go, Meking runs locally with SQLite storage. Completion and embedding models connect through configurable OpenAI-compatible APIs. Knowledge storage stays on your machine; configured model providers process the content sent to them.

## What you can do

- **Give agents persistent memory.** Save ordered messages and agent-extracted knowledge, then retrieve matching knowledge with its original evidence.
- **Track memory through natural recall.** Evaluate actual recall expressions against current knowledge, record the supporting evidence, and update memory state when the observation can be scored.
- **Track knowledge as it changes.** Keep stable object identities, explicit versions, and conflict candidates that can be reviewed and resolved.
- **Build knowledge from documents.** Import text and extract entities, relations, and optional claims. Add the analysis sidecar for PDF, DOCX, PPTX, XLSX, HTML, and sentence-based processing.
- **Ask questions across your knowledge.** Use Basic, Local, Global, or DRIFT search, with streaming answers and citations. Explore entities, communities, and generated reports in the Web UI.
- **Separate users and sessions.** Organize knowledge into Root and Child Zones, with agent memory bound to a user and session.

Meking draws on GraphRAG ideas for graph-based retrieval, community discovery, and report generation. It maintains its own knowledge model and processing lifecycle.

## Memory activation through natural recall

A conversation can provide evidence of successful recall, difficulty, or failure. Meking gives these observations an explicit evaluation and storage path, tied to the current version of a knowledge object.

The memory activation model draws on FSRS memory-state calculations:

| State | Meaning |
| --- | --- |
| Stability (S) | The time scale over which memory is retained, measured in days. |
| Difficulty (D) | How difficult it is to increase stability through recall. |
| Activation (R) | An estimate of retrievability at a given time, calculated from state and elapsed time. |

The host application fixes the observation's identity, target, time, and source material, including what was visible during the recall expression. An evaluator agent assesses the expression using the supplied protocol and cites its evidence. Meking validates the submission and applies a scorable observation once, saving the observation and state update in one transaction.

**The quality of the observation matters.** A search hit, citation, confirmation, or repeated mention does not automatically reinforce memory. The evaluation protocol requires checking whether the answer was already visible and whether the evidence supports a judgment. Successful recall with unknown difficulty is unscorable rather than assigned a default grade; unscorable observations are retained without updating memory state.

Observation history preserves the original assessment, time, protocol, and model identity. Retries do not apply the same observation twice, and startup verification replays historical inputs with their original model parameters to check stored state.

**Current scope:** observation evaluation contracts, state computation, persistence, and replay verification are implemented. Activation is not yet used to rank search results, and Meking does not schedule automatic reviews. The model estimates memory state; improvements to agent retrieval quality still require evaluation in real workloads.

## Quick start

You need Go 1.26+, Node.js 22.12+ with npm, and access to completion and embedding models.

```bash
git clone https://github.com/gorenx/meking.git
cd meking
make frontend-sync
make init ../meking-data
```

Initialization creates `settings.yaml`, `.env`, and prompt files in `../meking-data`.

1. Set `MEKING_API_KEY` in `../meking-data/.env`.
2. Review the completion and embedding models in `../meking-data/settings.yaml`. The defaults are `gpt-4.1` and `text-embedding-3-large`. You can configure separate providers, API base URLs, and credential references.
3. Start the service:

```bash
make start START_ROOT=../meking-data
```

Open **http://127.0.0.1:8080** for the Web UI. Create a Zone, upload a text document, and inspect processing progress before querying the resulting knowledge.

The default text/token setup does not require the Python sidecar. Configuration is loaded at startup; restart the service after changing settings or credentials.

## Connect an agent

The running HTTP service exposes a Streamable HTTP MCP endpoint:

```text
http://127.0.0.1:8080/api/v1/mcp
```

Configure this URL in your MCP client. Meking provides tools for:

| Capability | Tools |
| --- | --- |
| Save and retrieve memory | `add_memory`, `search_memory` |
| Review competing knowledge | `list_*_conflicts`, `get_*_conflict`, `resolve_*_conflict` for entities, relations, and claims |
| Delete knowledge | `delete_entity`, `delete_relation`, `delete_claim` |
| Evaluate natural recall | `get_recall_evaluation_protocol`, `submit_recall_observation` |

Memory tools take a `user_id` and UUID-formatted `session_id`. Keep these values stable for the intended user and conversation. Meking creates the corresponding User Root and Session Child Zone on first use.

Recall submission requires additional host integration: the host binds observation identity, target, time, and evidence outside the model-visible tool arguments. The agent supplies only its assessment and references to existing evidence. Read the protocol tool's schema before integrating this flow.

A built executable also supports STDIO transport:

```bash
./bin/meking mcp --root ../meking-data
```

## Optional document analysis

For rich documents or sentence-based processing, install Python 3.12 and `uv`, then prepare the sidecar:

```bash
make sidecar-sync
make sidecar-resources
```

Select the corresponding input or chunking mode in your Project settings. `make start` supplies the repository's sidecar executable to the service. Packages and language resources are prepared ahead of time.

## Build and test

Run these commands from the repository root. The full build requires the frontend dependencies and the sidecar prerequisites above.

```bash
make build          # Build the Go executable, Web UI, and sidecar environment
make test           # Run Go tests
make quality        # Run Go tests, race tests, and vet
make release-check  # Also verify sidecar tests and frontend artifacts
```

The Go executable is written to `bin/meking`.

## Deployment notes

- Project data persists locally in SQLite and the Project filesystem. Back up the Project directory along with its configuration.
- The service has no built-in authentication. Keep the default loopback binding, or place it behind an authenticated gateway when exposing it to a network. User and Session identifiers are not credentials.
- Completion and embedding calls use your configured provider credentials and may incur provider charges.
