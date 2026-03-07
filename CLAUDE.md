# CLAUDE.md — WebMux

## Project Conventions (from ai-template)

### Project Structure

Every project MUST have the following top-level directories and files:

- `tests/` — All test files go here (not scattered in `src/__tests__/`, `lib/test/`, etc.)
- `docs/` — All documentation goes here (architecture, setup guides, API docs, etc.)
- `README.md` — Unique to this project (see README section below)
- `Makefile` — With at least the required targets (see Makefile section below)
- `LICENSE` — Project license file

### README.md

Every project README must be **unique to the project** — not the generic ai-template README.
When creating or rewriting a README for a new project:

1. Write a project-specific README covering: what it is, features, quick start, configuration, development, and deployment.
2. **Include the PROVENANCE origin story.** Use the PROVENANCE skill (`skills/PROVENANCE.md`) to write a new chapter in the "Totally True and Not At All Embellished History" chronicle. This is mandatory for every AI-assisted project. The origin story goes near the end of the README, before the License section.
3. Follow the full PROVENANCE checklist: determine the next Part number, update the previous Part's forward links and project count, write the new chapter, and update the chronicle table in `skills/PROVENANCE.md`.

If the README still contains the generic ai-template text ("All skills and AI template files required to implement any of my repositories"), it has not been customized yet — fix it.

### Makefile

Every project MUST have a top-level `Makefile` with at least these targets:

| Target | Description |
|--------|-------------|
| `make` / `make all` | Default build (install deps + compile/bundle) |
| `make start` | Build and start the application |
| `make stop` | Stop the running application |
| `make restart` | Stop then start |
| `make test` | Run all tests |
| `make clean` | Remove build artifacts, deps, generated files |

Additional targets (`lint`, `status`, `configure`, `help`, etc.) are encouraged but not required.

### Testing

- **All tests MUST go in the `tests/` directory** at the top level of the repository.
- **Code coverage must be at least 70%.** When writing new code, write tests to cover it. When modifying existing code, check coverage and add tests if it falls below 70%.
- **All tests must pass before push.** Do not push code with failing tests. Run `make test` and verify before any push.
- Test files should mirror the source structure (e.g., `tests/backend/sessionBroker.test.ts` for `backend/src/services/sessionBroker.ts`).
- Use the project's existing test framework (Jest, Vitest, pytest, etc.) — do not introduce a second test runner.

### Code Quality

- Write safe, secure code. Follow OWASP top-10 guidelines.
- Do not introduce security vulnerabilities (command injection, XSS, SQL injection, etc.).
- Keep solutions simple. Do not over-engineer.
- Do not add features, refactoring, or "improvements" beyond what was requested.

### Skills

Reusable AI prompt templates live in `skills/`. See `skills/README.md` for the index.
When a skill applies to the current task, use it. Key skills:

- **PROVENANCE** — Write the project origin story chapter (mandatory for every new project)

---

## WebMux-Specific Notes

- The application source lives under `webmux/` (backend + frontend workspaces).
- Backend: Express + ws + node-pty (TypeScript). Frontend: React + xterm.js (TypeScript, Vite).
- Configuration is YAML-based in `webmux/config/`.
- The top-level `Makefile` delegates to `npm` scripts inside `webmux/`.
