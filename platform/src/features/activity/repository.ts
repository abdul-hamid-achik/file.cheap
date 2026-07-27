import type { PrivateActivityEvent } from "@/features/activity/contracts";

/**
 * Append-only by construction: the domain port exposes no update or delete
 * operation. A database adapter must grant the runtime role matching rights.
 */
export interface PrivateActivityLedgerRepository {
  append(event: PrivateActivityEvent): Promise<void>;
  recent(limit: number): Promise<PrivateActivityEvent[]>;
}
