import { createDockerDesktopClient } from "@docker/extension-api-client";

// Types mirror internal/governor in the Go backend.

export type Mode = "gpu" | "labeled" | "all";

export interface Limits {
  enabled: boolean;
  pauseAtC: number;
  resumeAtC: number;
  tripSamples: number;
  clearSamples: number;
}

export interface Config {
  enabled: boolean;
  mode: Mode;
  intervalSeconds: number;
  cpu: Limits;
  gpu: Limits;
}

export interface Reading {
  name: string;
  tempC: number;
}

export interface SensorStatus {
  available: boolean;
  readings: Reading[] | null;
  maxC: number;
  tripped: boolean;
  error?: string;
}

export interface ContainerStatus {
  id: string;
  name: string;
  image: string;
  state: string;
  usesGPU: boolean;
  target: boolean;
  pausedByGovernor: boolean;
}

export interface PausedContainer {
  id: string;
  name: string;
  since: string;
  reason: string;
}

export interface GovernorEvent {
  time: string;
  level: "info" | "warn" | "error";
  message: string;
}

export interface Status {
  hot: boolean;
  cpu: SensorStatus;
  gpu: SensorStatus;
  containers: ContainerStatus[] | null;
  paused: PausedContainer[] | null;
  events: GovernorEvent[] | null;
  config: Config;
  updatedAt: string;
  error?: string;
}

const ddClient = createDockerDesktopClient();
const service = ddClient.extension.vm?.service;

function backend() {
  if (!service) {
    throw new Error("The extension backend is not available.");
  }
  return service;
}

// The SDK rejects with the backend's JSON body; pull out a readable message.
export function errorMessage(err: unknown): string {
  if (err instanceof Error) return err.message;
  if (typeof err === "object" && err !== null) {
    const e = err as { message?: unknown; body?: unknown };
    if (typeof e.message === "string") return e.message;
    if (typeof e.body === "string") {
      try {
        const parsed = JSON.parse(e.body) as { message?: string };
        if (parsed.message) return parsed.message;
      } catch {
        return e.body;
      }
    }
  }
  return String(err);
}

export async function getStatus(): Promise<Status> {
  return (await backend().get("/status")) as Status;
}

export async function saveConfig(cfg: Config): Promise<Config> {
  return (await backend().post("/config", cfg)) as Config;
}

export async function resumeAll(): Promise<Status> {
  return (await backend().post("/resume", {})) as Status;
}

export function toast() {
  return ddClient.desktopUI.toast;
}
