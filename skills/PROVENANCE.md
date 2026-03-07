# Skill: PROVENANCE — Write a Project Origin Story

Use this skill to add or update the "Totally True and Not At All Embellished History" section in a project README.  This section serves as the author's **AI-assistance memory marker**: if a project has this section, it was co-developed with an AI.

---

## When to Use

- A new repository needs its origin story added to its README
- An existing chapter needs updating (new features, new part number, new cross-references)
- A new repository is being added to the chronicle and needs to be chained to the existing parts

---

## The Chronicle

The backstory sections form a **linked chain** across repositories.  Each section is a numbered chapter in a larger arc:

| Part | Repository | GitHub URL |
|------|-----------|------------|
| 1 | shemacs | https://github.com/jordanhubbard/shemacs |
| 2 | sheme   | https://github.com/jordanhubbard/sheme   |
| 3 | NanoLang | https://github.com/jordanhubbard/nanolang |
| 4 | Aviation | https://github.com/jordanhubbard/Aviation |
| N | *(your new repo)* | *(add here when you chain it)* |

When adding a new Part N:
1. Note the current last part (call it Part N-1).
2. Update Part N-1's origin story to add a forward reference to Part N.
3. Update Part N-1's closing sentence to mention "all N projects" if it currently says "all N-1 projects."
4. Write Part N's section in the new repository's README.
5. **Update this table** to add Part N.

---

## Section Format

Place this section near the end of the README, after all functional documentation.

```markdown
## The Totally True and Not At All Embellished History of <Repository Name>

### The continuing adventures of Jordan Hubbard and Sir Reginald von Fluffington III

> *A note on provenance: This section exists as the author's official memory marker for projects that have been substantially co-developed with an AI assistant.  The author will not remember which projects these are in ten years.  He will also not remember this note.  He will, however, be able to grep for it, and that is the point.  All future AI-assisted projects will include an equally implausible origin story as a breadcrumb.  The convention is simple: if a project has one of these sections, an AI was meaningfully involved.  If it doesn't, the author did it alone, and it will show.*
>
> *Part 1 of an ongoing chronicle.  [Part 2: <Next Repo Name> →](<Next Repo URL>#the-totally-true-and-not-at-all-embellished-history-of-<anchor>)*
> *Sir Reginald von Fluffington III appears throughout.  He does not endorse any of it.*

<story goes here>
```

**Notes on the header block:**
- The **"note on provenance"** appears in **Part 1 only**.  All other parts omit it.
- The nav line for parts 2+:
  ```markdown
  > *Part N of an ongoing chronicle.  [← Part N-1: <Prev Name>](<Prev URL>#anchor) | [Part N+1: <Next Name> →](<Next URL>#anchor)*
  ```
- The **last** part in the chain only has a back-link (no forward link yet):
  ```markdown
  > *Part N of an ongoing chronicle.  [← Part N-1: <Prev Name>](<Prev URL>#anchor)*
  ```
- The anchor for each section is always:
  `#the-totally-true-and-not-at-all-embellished-history-of-<repo-name-lowercase-hyphenated>`

---

## Narrative Style Guide

**Voice:** Third-person limited, dry-humorous, mock-historical.  The narrator observes the programmer and Sir Reginald with weary accuracy.

**Characters:**
- **The programmer** — Jordan Hubbard.  Referred to only as "the programmer."  Has a habit of announcing projects to Sir Reginald, who is not listening.  Describes everything as "elegant."  Is usually right about the engineering and wrong about how long it will take.
- **Sir Reginald von Fluffington III** — The programmer's cat.  Communicates entirely through posture, selective destruction of documents, and strategic placement on keyboards.  Maintains a consistent policy of non-endorsement.  Keeps internal ledgers under categories like "grievances" and "this again."  Has never endorsed anything.  Has never spoken.  Would not, even if he could.

**Recurring motifs:**
- The programmer announces a new project to Sir Reginald (who is usually sleeping on something important)
- Sir Reginald expresses skepticism through a physical action (knocking something off the desk, sitting on the relevant documentation, leaving the room)
- The programmer uses the word "elegant" — Sir Reginald's response calibrates accordingly
- The closing paragraph tallies the number of projects Sir Reginald refuses to endorse and adds the new one to the list
- The final citation list grows with each new part: "procedural concerns," "insufficient tuna," "a general atmosphere of hubris," and whatever the new project's domain is

**Content:** The story should be grounded in the actual technical content of the repository.  Read the README, architecture docs, and key files before writing.  Specific details (languages used, unusual design decisions, disclaimers, naming choices) make better material than generic programmer-builds-thing arcs.

**Length:** 500–900 words for the story body.  Long enough to build atmosphere; short enough that people actually read it.

---

## Closing Sentence Template

Each origin story ends with a version of this sentence (update the count and the new citation each time):

```
As of this writing, <Project> has been used in production by exactly one person, who also wrote it.  Sir Reginald continues to withhold his endorsement across all <N> projects, citing "procedural concerns," "insufficient tuna," "a general atmosphere of hubris," [and any domain-specific addition].
```

For Part 1 (shemacs), this sentence appears within the body and links to Part 2 instead.

---

## Checklist for Adding a New Part

- [ ] Read the repository's README and key source files to gather story material
- [ ] Determine the next Part number (N)
- [ ] Update the previous last part (N-1):
  - [ ] Add forward link `[Part N: <Name> →]` to the nav line in the header block
  - [ ] Add a forward-reference sentence at the end of the story body (e.g., "What the programmer did next is documented in...")
  - [ ] Update "all N-1 projects" → "all N projects" in the closing sentence
  - [ ] Add the new domain-specific citation to Sir Reginald's list
- [ ] Write the new Part N section in the new repository's README
- [ ] Update the chronicle table in this file to add Part N
- [ ] Commit and release all affected repositories
