import type { Blocker } from "react-router-dom";

export function resolvePendingDBNavigation(blocker: Blocker, committing: boolean, confirmDiscard: () => boolean) {
  if (blocker.state !== "blocked") return;
  if (!committing && confirmDiscard()) blocker.proceed();
  else blocker.reset();
}
