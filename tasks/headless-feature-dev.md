# Headless Feature-Dev: Autonomous Agent-Parallel Task Execution

> Combines Ralph's zero-interaction autonomous loop with feature-dev's parallel sub-agent architecture.
> Designed for headless (non-interactive) Claude Code sessions.

## Problem

| Approach | Strength | Weakness |
|----------|----------|----------|
| **Ralph (CLAUDE.ralph.md)** | Fully autonomous, PRD-driven, no interaction gates | Single-threaded reasoning — no parallel exploration or review |
| **feature-dev skill** | Parallel agents (explorer, architect, reviewer) | 4 mandatory user interaction points — unusable headless |

**Goal**: A prompt/skill that runs autonomously like Ralph but leverages parallel agents like feature-dev for higher-quality implementation.

---

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                  Orchestrator Loop                   │
│  (main agent — reads PRD, picks story, commits)     │
├─────────────────────────────────────────────────────┤
│                                                     │
│  1. Read PRD + progress.txt                         │
│  2. Pick highest-priority failing story             │
│  3. ┌──────────────────────────────────────┐        │
│     │  Phase: Explore (parallel agents)    │        │
│     │  2-3 code-explorer agents            │        │
│     │  - Trace related features            │        │
│     │  - Map dependencies                  │        │
│     │  - Identify files to modify          │        │
│     └──────────────────────────────────────┘        │
│  4. Read identified files                           │
│  5. ┌──────────────────────────────────────┐        │
│     │  Phase: Plan (single architect)      │        │
│     │  1 code-architect agent              │        │
│     │  - Constrained by CLAUDE.md          │        │
│     │  - PRD acceptance criteria as input   │        │
│     │  - Returns file-level impl plan      │        │
│     └──────────────────────────────────────┘        │
│  6. Implement (orchestrator writes code)            │
│  7. Run quality checks (make test && make lint)     │
│  8. ┌──────────────────────────────────────┐        │
│     │  Phase: Review (parallel agents)     │        │
│     │  2-3 code-reviewer agents            │        │
│     │  - Bugs/correctness                  │        │
│     │  - Conventions/patterns              │        │
│     │  - Simplicity/quality                │        │
│     └──────────────────────────────────────┘        │
│  9. Auto-fix critical findings (confidence ≥ 75)    │
│ 10. Re-run quality checks if fixes applied          │
│ 11. Commit + update PRD + append progress.txt       │
│                                                     │
└─────────────────────────────────────────────────────┘
```

---

## Phase Details

### Phase 1: Story Selection (no agents)

Same as Ralph — deterministic, no interaction needed.

```
1. Read prd.json
2. Read progress.txt (especially Codebase Patterns section)
3. Ensure correct branch
4. Pick highest-priority story where passes: false
5. Extract: story ID, title, acceptance criteria, technical context
```

### Phase 2: Explore (parallel agents)

Replace Ralph's implicit "read around and figure it out" with structured parallel exploration.

**Spawn 2-3 `code-explorer` agents via Task tool**, each with a different focus:

| Agent | Focus | Prompt seed |
|-------|-------|-------------|
| Explorer A | **Similar features** — find existing code that does something like this story | "Find existing implementations similar to: {story.title}. Trace execution paths, map layers, list 5-10 key files." |
| Explorer B | **Dependencies** — what does this story's target area depend on? | "Trace the dependency graph for: {story.targetPackages}. Map imports, interfaces, shared types. List 5-10 key files." |
| Explorer C | **Test patterns** — how are similar features tested? | "Find test files for features similar to: {story.title}. Document test patterns, mock strategies, fixture conventions. List 5-10 key files." |

**Key difference from feature-dev**: No user Q&A after exploration. The orchestrator reads the identified files and moves on.

**Decision rule**: Union of all identified files → read them all. If > 20 files, prioritize by frequency (files mentioned by multiple agents).

### Phase 3: Plan (single architect agent)

Feature-dev spawns 3 architects with different philosophies and asks the user to pick. Headless mode doesn't need that — the architecture is already constrained by CLAUDE.md conventions.

**Spawn 1 `code-architect` agent** with tight constraints:

```
Prompt:
  Design the implementation for story {story.id}: {story.title}

  Acceptance criteria:
  {story.acceptanceCriteria}

  Constraints (non-negotiable):
  - Follow the command constructor pattern from CLAUDE.md
  - Follow the dependency layering from CLAUDE.md
  - Follow existing test patterns (httptest, iostreams.Test, table-driven)
  - Minimize new files — prefer extending existing ones
  - No new dependencies unless absolutely required

  Context from exploration:
  {explorer_results_summary}

  Output:
  - Files to create (with full path)
  - Files to modify (with what changes)
  - Build sequence (ordered steps)
  - Key interfaces/types needed
```

**Decision rule**: Accept the architect's plan as-is. No user choice needed because CLAUDE.md already eliminates most architectural freedom.

### Phase 4: Implement (orchestrator, no agents)

The main agent implements the plan. This is the same as Ralph — the orchestrator writes the code directly.

Why not delegate implementation to agents? Because:
- Implementation requires sequential file edits with dependencies between them
- The orchestrator has full conversation context from exploration + planning
- Sub-agents would need the full context passed in, negating the benefit

### Phase 5: Quality Checks (shell commands)

```sh
make build   # compilation check
make test    # all tests pass
make lint    # go vet clean
```

If checks fail → fix and re-run (same as Ralph).

### Phase 6: Review (parallel agents, optional)

**Spawn 2-3 `code-reviewer` agents** looking at `git diff` of unstaged changes:

| Agent | Focus |
|-------|-------|
| Reviewer A | **Bugs & correctness** — logic errors, nil handling, race conditions, security |
| Reviewer B | **Conventions** — CLAUDE.md adherence, existing patterns, naming, structure |
| Reviewer C | **Simplicity** — DRY violations, unnecessary complexity, dead code |

**Decision rule (fully autonomous)**:
- **Critical findings (confidence ≥ 75)**: Auto-fix, then re-run Phase 5
- **Medium findings (confidence 50-74)**: Log in progress.txt as "known debt", do not fix
- **Low findings (confidence < 50)**: Ignore

**Skip condition**: If the story is trivial (e.g., < 50 lines changed), skip review phase entirely to save tokens.

### Phase 7: Commit & Report (no agents)

Same as Ralph:
```
1. git add <specific files>
2. git commit -m "feat: [Story ID] - [Story Title]"
3. Update prd.json: passes → true
4. Append to progress.txt (including learnings from review agents)
5. Update CLAUDE.md if reusable patterns discovered
```

---

## When to Use Which Phase

Not every story needs every phase. Token budget matters in headless mode.

| Story complexity | Phases to run |
|-----------------|---------------|
| **Trivial** (< 30 lines, single file) | 1 → 4 → 5 → 7 |
| **Standard** (single package, clear pattern) | 1 → 2 (2 explorers) → 4 → 5 → 7 |
| **Complex** (multi-package, new patterns) | 1 → 2 (3 explorers) → 3 → 4 → 5 → 6 (3 reviewers) → 7 |

The orchestrator decides complexity based on:
- Number of acceptance criteria (> 5 = complex)
- Whether the story introduces a new package/pattern
- Whether similar code already exists (if yes, simpler)

---

## Prompt Template (Orchestrator)

Below is the core prompt for the headless orchestrator. It replaces `CLAUDE.ralph.md`.

```markdown
You are an autonomous coding agent. You work in headless mode — no user interaction.

## Your Task

1. Read `prd.json` and `progress.txt` (Codebase Patterns section first)
2. Ensure correct branch from PRD `branchName`
3. Pick the highest-priority story where `passes: false`
4. Assess complexity (trivial / standard / complex)
5. Execute the appropriate phases (see below)
6. Commit all changes with message: `feat: [Story ID] - [Story Title]`
7. Update PRD (`passes: true`) and append to `progress.txt`

## Phase Execution

### Explore (standard + complex stories)
Launch 2-3 code-explorer agents in parallel via the Task tool:
- Agent A: Find similar features, trace execution paths
- Agent B: Map dependencies of the target area
- Agent C (complex only): Find test patterns for similar features

Read all files identified by agents (deduplicate, cap at 20).

### Plan (complex stories only)
Launch 1 code-architect agent with story acceptance criteria,
CLAUDE.md constraints, and exploration results. Accept the plan.

### Implement
Write the code yourself. Follow the plan if one was produced.
Follow CLAUDE.md conventions strictly.

### Quality Check
Run: make build && make test && make lint
Fix any failures. Re-run until green.

### Review (complex stories only)
Launch 2-3 code-reviewer agents in parallel on `git diff`:
- Auto-fix findings with confidence >= 75
- Log findings with confidence 50-74 in progress.txt
- Ignore findings with confidence < 50
Re-run quality checks after fixes.

### Commit
- git add <specific files>
- git commit (conventional commit format)
- Update prd.json and progress.txt

## Decision Rules (replaces human interaction)

- Architecture choices: CLAUDE.md conventions are the decision
- Ambiguous requirements: PRD acceptance criteria are the spec
- Multiple valid approaches: Pick the simplest that passes all criteria
- Review findings: Auto-fix critical, log medium, ignore low

## Stop Condition

If all stories pass: reply with <promise>COMPLETE</promise>
Otherwise: end normally (next iteration picks up).
```

---

## Comparison: Ralph vs Headless Feature-Dev

| Dimension | Ralph | Headless Feature-Dev |
|-----------|-------|---------------------|
| Exploration | Implicit (agent reads files as needed) | Explicit parallel agents, structured output |
| Planning | None (CLAUDE.md is the plan) | Optional architect agent for complex stories |
| Implementation | Orchestrator | Orchestrator (same) |
| Quality checks | `make test && make lint` | Same + parallel reviewer agents |
| Review | None | Parallel code-reviewer agents (complex stories) |
| Token cost | Low (~50k per story) | Medium-High (~100-200k per story) |
| Quality | Good (conventions enforced) | Higher (multi-perspective review) |
| Speed | Fast (single-threaded) | Slower (agent spawning overhead) |
| Complexity ceiling | Standard stories | Complex multi-package stories |

---

## Implementation Path

To actually build this:

1. **Phase 1 — Use sub-agents in Ralph prompt**: Modify `CLAUDE.ralph.md` to spawn `code-explorer` and `code-reviewer` agents via Task tool for complex stories. No new skill needed.

2. **Phase 2 — Create a skill**: If Phase 1 proves valuable, extract into a proper `/headless-dev` skill with the orchestrator prompt as the command template and the complexity assessment as a pre-step.

3. **Phase 3 — Adaptive token budgeting**: Track token usage per story in progress.txt. If a story consistently completes under budget with review, always review. If over budget, skip review for trivial stories.

---

## Open Questions

- **Token budget per story**: What's the ceiling before headless mode hits context limits? Sub-agents help here by offloading exploration to separate contexts.
- **Agent model selection**: Explorers and reviewers can run on `haiku` for cost savings. Architect should stay on `sonnet` or `opus`.
- **Failure cascades**: If a reviewer finds something the orchestrator can't auto-fix, should it skip commit and flag for the next iteration? (Current answer: yes, log it and move on.)
