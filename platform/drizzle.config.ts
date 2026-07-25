import { defineConfig } from "drizzle-kit";

if (!process.env.MIGRATIONS_DATABASE_URL) {
  throw new Error("MIGRATIONS_DATABASE_URL is required for Drizzle migrations");
}

export default defineConfig({
  dialect: "postgresql",
  out: "./drizzle",
  schema: "./src/platform/database/schema.ts",
  dbCredentials: { url: process.env.MIGRATIONS_DATABASE_URL },
});
