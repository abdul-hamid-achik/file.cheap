import { neon } from "@neondatabase/serverless";
import { drizzle } from "drizzle-orm/neon-http";

import { getConfig } from "@/shared/config/env";
import * as schema from "@/platform/database/schema";

let database: ReturnType<typeof drizzle<typeof schema>> | undefined;

export function getDatabase() {
  database ??= drizzle(neon(getConfig().databaseUrl), { schema });
  return database;
}

export function resetDatabaseForTests(): void { database = undefined; }
