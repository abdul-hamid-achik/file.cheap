import {
  privateActivityEventSchema,
  type PrivateActivityEvent,
} from "@/features/activity/contracts";
import type { PrivateActivityLedgerRepository } from "@/features/activity/repository";

export class InMemoryPrivateActivityLedgerRepository implements PrivateActivityLedgerRepository {
  private readonly events: PrivateActivityEvent[] = [];

  async append(input: PrivateActivityEvent): Promise<void> {
    const event = privateActivityEventSchema.parse(input);
    if (this.events.some((existing) => existing.eventId === event.eventId)) {
      throw new Error("The private activity event already exists");
    }
    this.events.push(cloneEvent(event));
  }

  async recent(limit: number): Promise<PrivateActivityEvent[]> {
    if (!Number.isSafeInteger(limit) || limit < 1 || limit > 100) {
      throw new Error("Activity history limits must be integers from 1 through 100");
    }
    return [...this.events]
      .sort((left, right) => {
        const timeDifference = right.recordedAt.getTime() - left.recordedAt.getTime();
        return timeDifference === 0
          ? right.eventId.localeCompare(left.eventId)
          : timeDifference;
      })
      .slice(0, limit)
      .map(cloneEvent);
  }
}

function cloneEvent(event: PrivateActivityEvent): PrivateActivityEvent {
  return structuredClone(event);
}
