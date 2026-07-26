import { RunService } from "@/features/runs/service";
import { DrizzleRunRepository } from "@/platform/database/run-repository";

let service: RunService | undefined;

export function getRunService(): RunService {
  service ??= new RunService(new DrizzleRunRepository());
  return service;
}

export function setRunServiceForTests(value?: RunService): void { service = value; }
