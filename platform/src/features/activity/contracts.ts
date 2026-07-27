import { z } from "zod";

import {
  retentionCountersSchema,
  retentionFailureAreaSchema,
  retentionRunIdSchema,
  retentionRunStatusSchema,
} from "@/features/retention/contracts";

const activityIdSchema = z.string().regex(
  /^act_[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/,
  "Activity IDs must be generated opaque identifiers",
);

const activityEnvelopeSchema = z.object({
  actor: z.literal("system:retention"),
  eventId: activityIdSchema,
  recordedAt: z.date(),
  subject: z.object({
    id: retentionRunIdSchema,
    type: z.literal("retention_run"),
  }).strict(),
}).strict();

const startedEventSchema = activityEnvelopeSchema.extend({
  details: z.object({}).strict(),
  eventName: z.literal("private.retention_run.started"),
}).strict();

const terminalDetailsSchema = z.object({
  counters: retentionCountersSchema,
  failedAreas: z.array(retentionFailureAreaSchema).max(8),
  oldestDueAt: z.date().nullable(),
  status: retentionRunStatusSchema.exclude(["running"]),
}).strict();

const terminalEventSchema = activityEnvelopeSchema.extend({
  details: terminalDetailsSchema,
  eventName: z.enum([
    "private.retention_run.succeeded",
    "private.retention_run.partial",
    "private.retention_run.failed",
    "private.retention_run.abandoned",
  ]),
}).strict().superRefine((event, context) => {
  const eventStatus = event.eventName.split(".").at(-1);
  if (eventStatus !== event.details.status) {
    context.addIssue({
      code: "custom",
      message: "The terminal event name must match the retention run status",
      path: ["details", "status"],
    });
  }
});

/**
 * Private activity events deliberately have no free-form actor, message, or
 * metadata field. Adding another event requires extending this discriminated
 * contract with an explicit detail allowlist.
 */
export const privateActivityEventSchema = z.union([
  startedEventSchema,
  terminalEventSchema,
]);

export type PrivateActivityEvent = z.infer<typeof privateActivityEventSchema>;
