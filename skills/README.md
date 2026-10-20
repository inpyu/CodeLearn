# CodeLearn Shared Skill

`codelearn/` holds the agent-independent skill that writes the learning
explanations CodeLearn displays. The same `SKILL.md` is used by every agent;
only the install location differs.

## Claude Code

Copy the skill into either scope:

```bash
# this project only
mkdir -p .claude/skills && cp -r skills/codelearn .claude/skills/

# every project
mkdir -p ~/.claude/skills && cp -r skills/codelearn ~/.claude/skills/
```

Then ask Claude Code to analyse the current changes with CodeLearn.

## Codex and other agents

Agents that do not read `SKILL.md` files from a conventional directory can be
pointed at the file directly:

```text
Follow the instructions in skills/codelearn/SKILL.md to analyse the
uncommitted changes in this repository.
```

The YAML front matter is metadata for skill discovery; the Markdown body is a
self-contained procedure, so any agent that can run shell commands and write a
file can execute it. The output must match
`codelearn/references/analysis-schema.json`. CodeLearn enforces that contract
in Go rather than with a JSON Schema library, so the schema file is the
agent-facing document; `codelearn import` is the authority.

## Privacy

The skill runs inside your agent, which means the code it reads is subject to
that agent's data handling. CodeLearn itself sends nothing anywhere.
