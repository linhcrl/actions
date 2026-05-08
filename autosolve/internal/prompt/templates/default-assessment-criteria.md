- PROCEED if: the task is clear, affects a bounded set of files, can be
  delivered as a single commit, and does not require architectural decisions
  or human judgment on product direction.
- SKIP if: the task is ambiguous, requires design decisions or RFC, affects
  many unrelated components, requires human judgment, or would benefit from
  being split into multiple commits (e.g., separate refactoring from
  behavioral changes, or independent fixes across unrelated subsystems).
