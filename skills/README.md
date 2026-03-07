# Skills Index

Reusable AI skills for use with [Claude Code](https://claude.ai/claude-code).  Each skill is a prompt template that can be invoked in a project to apply a consistent pattern.

## Available Skills

| Skill | Description |
|-------|-------------|
| [PROVENANCE.md](PROVENANCE.md) | Write a humorous project origin story chapter and chain it into the "Totally True and Not At All Embellished History" chronicle.  Includes style guide, character notes, nav link format, and a checklist for adding a new Part. |

## How to Use a Skill

Copy the skill file into your project's `.claude/` directory, or reference it directly when prompting Claude Code:

```
Use the PROVENANCE skill from ~/Src/ai-template/skills/PROVENANCE.md to write the origin story for this repository.
```

Or, if the ai-template repo is linked as a Claude Code skill source, invoke it with:

```
/PROVENANCE
```

## Adding a New Skill

1. Create a new `.md` file in this directory named after the skill.
2. Include: when to use it, the pattern/template it applies, style notes, and a checklist.
3. Add it to the table above.
4. Commit and push.
