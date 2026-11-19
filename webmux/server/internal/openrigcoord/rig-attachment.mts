import path from 'node:path';

export interface RigAttachmentNode {
  canonicalSessionName?: string; attachmentType?: string | null;
  tmuxAttachCommand?: string | null; tmuxServerArgs?: unknown;
}
export interface RigTmuxAttachment { tmuxTarget: string; tmuxServerFlag: string; tmuxServerValue: string }
const sessionName = /^[a-zA-Z0-9_.:@-]{1,256}$/;

// Only the qualified structured contract can choose a different local server.
// The human attach command is never executed or parsed as a shell program.
export function rigTmuxAttachment(node: RigAttachmentNode, structured: boolean, uid = process.getuid?.()): RigTmuxAttachment | undefined {
  const target = node.canonicalSessionName;
  if (!target || !sessionName.test(target)) return;
  if (!structured) {
    const match = typeof node.tmuxAttachCommand === 'string' ? /^tmux attach(?:-session)? -t ([a-zA-Z0-9_.:@-]{1,256})$/.exec(node.tmuxAttachCommand) : null;
    if (match?.[1] !== target) return;
    return { tmuxTarget: target, tmuxServerFlag: '', tmuxServerValue: '' };
  }
  if (node.attachmentType !== 'tmux') return;
  const args = node.tmuxServerArgs === undefined ? [] : node.tmuxServerArgs;
  if (!Array.isArray(args)) return;
  if (!args.length) {
    // In this contract an omitted selection names tmux's standard default
    // server. Do not let an attaching controller's TMUX/TMUX_TMPDIR redirect it.
    if (!Number.isSafeInteger(uid) || uid === undefined || uid < 0) return;
    return { tmuxTarget: target, tmuxServerFlag: '-S', tmuxServerValue: `/tmp/tmux-${uid}/default` };
  }
  if (args.length !== 2 || typeof args[0] !== 'string' || typeof args[1] !== 'string') return;
  const [flag, value] = args;
  if (!value || value.length > 2048 || /\p{Cc}/u.test(value)) return;
  if (flag === '-S' && path.isAbsolute(value)) {
    return { tmuxTarget: target, tmuxServerFlag: flag, tmuxServerValue: value };
  }
  // The qualified daemon uses -L only under its standard /tmp namespace.
  // Resolve that label too, so a different attaching environment cannot move it.
  if (flag === '-L' && /^[a-zA-Z0-9_.-]{1,128}$/.test(value) && value !== '.' && value !== '..'
    && uid !== undefined && Number.isSafeInteger(uid) && uid >= 0) {
    return { tmuxTarget: target, tmuxServerFlag: '-S', tmuxServerValue: `/tmp/tmux-${uid}/${value}` };
  }
}
