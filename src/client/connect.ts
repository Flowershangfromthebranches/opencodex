import { createHash } from "node:crypto";
import {
  existsSync,
  lstatSync,
  readFileSync,
  unlinkSync,
} from "node:fs";
import { hostname } from "node:os";
import { atomicWriteFile, loadConfig } from "../config";
import { invalidateCodexModelsCache } from "../codex/catalog/sync";
import {
  injectCodexConfig,
  currentExternalCodexModelProvider,
  isCodexRoutingInjected,
  type CodexRoutingTarget,
} from "../codex/inject";
import {
  journalOwner,
  restoreJournalState,
} from "../codex/journal";
import { DEFAULT_CATALOG_PATH } from "../codex/paths";
import {
  readServiceApiTokenState,
  readTokenBackupState,
  removeServiceApiTokenFileIfOwned,
  removeOrphanTokenBackup,
  replaceServiceApiTokenFile,
  restoreTokenBackup,
  serviceApiTokenBackupPath,
  writeTokenBackup,
  writeServiceApiTokenFile,
} from "../lib/service-secrets";
import { MAX_REMOTE_CATALOG_BYTES } from "../server/catalog-download";
import type {
  OcxClientConnectionConfig,
  OcxConnectedClientId,
} from "../types";
import {
  downloadClientCatalog,
  abortClientKeyRotation,
  commitClientKeyRotation,
  exchangeConnectPairingGrant,
  fetchHubReady,
  HubClientError,
  issueClientKey,
  normalizeHubOrigin,
  probeClientKeyId,
  revokeClientKey,
  startClientKeyRotation,
  type ConnectGuiSession,
  type IssuedClientKey,
  type OneTimeConnectCredential,
} from "./hub-client";
import {
  clearClientConnection,
  commitClientConnection,
  readClientConnectionState,
} from "./state";

class RotationRecoveryRequiredError extends Error {
  constructor(message: string, options?: ErrorOptions) {
    super(message, options);
    this.name = "RotationRecoveryRequiredError";
  }
}

export interface ConnectOptions {
  serverUrl: string;
  managementUrl?: string;
  credential: OneTimeConnectCredential;
  selectedClients: OcxConnectedClientId[];
  managementTransport: "direct" | "relay";
  noSync?: boolean;
  allowInsecureHttp?: boolean;
}

export interface ClientConnectDeps {
  fetchImpl?: typeof fetch;
  now?: () => Date;
}

export interface RotateClientOptions {
  credential: OneTimeConnectCredential;
  allowInsecureHttp?: boolean;
}

type CatalogSnapshot =
  | { kind: "absent" }
  | { kind: "file"; body: string; fingerprint: string };

function sha256(value: string): string {
  return createHash("sha256").update(value).digest("hex");
}

function catalogSnapshot(): CatalogSnapshot {
  if (!existsSync(DEFAULT_CATALOG_PATH)) return { kind: "absent" };
  const stat = lstatSync(DEFAULT_CATALOG_PATH);
  if (stat.isSymbolicLink() || !stat.isFile() || stat.size > MAX_REMOTE_CATALOG_BYTES) {
    throw new Error("existing OpenCodex catalog is not a bounded regular file");
  }
  const body = readFileSync(DEFAULT_CATALOG_PATH, "utf8");
  return { kind: "file", body, fingerprint: sha256(body) };
}

function restoreCatalogSnapshot(snapshot: CatalogSnapshot, writtenFingerprint: string): boolean {
  try {
    if (!existsSync(DEFAULT_CATALOG_PATH)) return snapshot.kind === "absent";
    const stat = lstatSync(DEFAULT_CATALOG_PATH);
    if (stat.isSymbolicLink() || !stat.isFile() || stat.size > MAX_REMOTE_CATALOG_BYTES) return false;
    const current = readFileSync(DEFAULT_CATALOG_PATH, "utf8");
    if (sha256(current) !== writtenFingerprint) return false;
    if (snapshot.kind === "absent") unlinkSync(DEFAULT_CATALOG_PATH);
    else atomicWriteFile(DEFAULT_CATALOG_PATH, snapshot.body);
    return true;
  } catch {
    return false;
  }
}

function validLocalCatalog(): string {
  const snapshot = catalogSnapshot();
  if (snapshot.kind !== "file") throw new Error("connected catalog is missing");
  try {
    const parsed = JSON.parse(snapshot.body) as unknown;
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) throw new Error("invalid");
  } catch {
    throw new Error("connected catalog is malformed");
  }
  return snapshot.body;
}

function catalogMatchesEtag(body: string, etag: string | undefined): boolean {
  if (!etag) return false;
  const digest = createHash("sha256").update(body).digest("base64url");
  return etag === `"sha256-${digest}"` || etag === `W/"sha256-${digest}"`;
}

function routingTarget(serverUrl: string): CodexRoutingTarget {
  return {
    baseUrl: `${serverUrl}/v1`,
    requiresAdmissionToken: true,
    tokenEnv: "OPENCODEX_API_AUTH_TOKEN",
  };
}

function localGuiOrigin(): string {
  const port = loadConfig().port;
  return `http://localhost:${Number.isInteger(port) && port > 0 ? port : 10100}`;
}

function clientKeyName(): string {
  const raw = `ocx connect ${hostname() || "client"}`;
  return raw.slice(0, 80);
}

function releaseCredential(credential: OneTimeConnectCredential): void {
  credential.value.fill(0);
}

async function rotationAuthority(
  connection: OcxClientConnectionConfig,
  options: RotateClientOptions,
  deps: ClientConnectDeps,
): Promise<{ kind: "admin"; value: Uint8Array } | { kind: "gui-session"; value: ConnectGuiSession }> {
  if (options.credential.kind === "admin") return { kind: "admin", value: options.credential.value };
  const session = await exchangeConnectPairingGrant(
    connection.managementUrl,
    localGuiOrigin(),
    options.credential.value,
    { allowInsecureHttp: options.allowInsecureHttp, fetchImpl: deps.fetchImpl },
  );
  return { kind: "gui-session", value: session };
}

function clearRotationState(
  connection: OcxClientConnectionConfig,
  tokenFingerprint: string,
): OcxClientConnectionConfig {
  const next = { ...connection, tokenFingerprint };
  delete next.pendingOperation;
  commitClientConnection(next);
  return next;
}

async function recoverRotationWithAuthority(
  connection: OcxClientConnectionConfig,
  authority: { kind: "admin"; value: Uint8Array } | { kind: "gui-session"; value: ConnectGuiSession },
  deps: ClientConnectDeps,
): Promise<OcxClientConnectionConfig> {
  const pending = connection.pendingOperation;
  if (!pending || pending.oldKeyBackupPath !== serviceApiTokenBackupPath()) {
    throw new RotationRecoveryRequiredError("rotation recovery state is missing or invalid");
  }
  const current = readServiceApiTokenState();
  const backup = readTokenBackupState();
  if (current.kind !== "present" || backup.kind !== "present") {
    throw new RotationRecoveryRequiredError(
      "rotation recovery requires owner-only current and .prev token files; preserve both and rerun ocx connect rotate with transient authority",
    );
  }
  let currentAccepted: boolean;
  let backupAccepted: boolean;
  try {
    [currentAccepted, backupAccepted] = await Promise.all([
      probeClientKeyId(connection.serverUrl, current.token, connection.apiKeyId, { fetchImpl: deps.fetchImpl }),
      probeClientKeyId(connection.serverUrl, backup.token, connection.apiKeyId, { fetchImpl: deps.fetchImpl }),
    ]);
  } catch (error) {
    throw new RotationRecoveryRequiredError(
      "rotation recovery could not establish both key admissions; preserve service-api-token and .prev",
      { cause: error },
    );
  }
  if (currentAccepted && backupAccepted) {
    await commitClientKeyRotation(connection.managementUrl, authority, connection.apiKeyId, pending.rotationId, { fetchImpl: deps.fetchImpl });
    const next = clearRotationState(connection, current.fingerprint);
    removeOrphanTokenBackup();
    return next;
  }
  if (currentAccepted && !backupAccepted) {
    const next = clearRotationState(connection, current.fingerprint);
    removeOrphanTokenBackup();
    return next;
  }
  if (!currentAccepted && backupAccepted) {
    const restored = restoreTokenBackup(pending.oldKeyBackupPath);
    await abortClientKeyRotation(connection.managementUrl, authority, connection.apiKeyId, pending.rotationId, { fetchImpl: deps.fetchImpl });
    const next = clearRotationState(connection, restored.fingerprint);
    removeOrphanTokenBackup();
    return next;
  }
  throw new RotationRecoveryRequiredError(
    "both rotation candidates were rejected; preserve service-api-token and .prev and repair admission from the hub",
  );
}

export async function recoverPendingClientRotation(
  options: RotateClientOptions,
  deps: ClientConnectDeps = {},
): Promise<OcxClientConnectionConfig> {
  try {
    const state = readClientConnectionState();
    if (state.kind !== "connected" || !state.value.pendingOperation) {
      throw new Error("no pending client key rotation to recover");
    }
    const authority = await rotationAuthority(state.value, options, deps);
    return await recoverRotationWithAuthority(state.value, authority, deps);
  } finally {
    releaseCredential(options.credential);
  }
}

export async function rotateConnectedClientKey(
  options: RotateClientOptions,
  deps: ClientConnectDeps = {},
): Promise<OcxClientConnectionConfig> {
  let connection: OcxClientConnectionConfig | null = null;
  let authority: { kind: "admin"; value: Uint8Array } | { kind: "gui-session"; value: ConnectGuiSession } | null = null;
  let started: { rotationId: string; key: string; createdAt: string } | null = null;
  let markerPersisted = false;
  try {
    const state = readClientConnectionState();
    if (state.kind !== "connected") throw new Error(`connect rotate is available only while connected (${state.kind})`);
    connection = state.value;
    authority = await rotationAuthority(connection, options, deps);
    if (connection.pendingOperation) return await recoverRotationWithAuthority(connection, authority, deps);
    const current = readServiceApiTokenState();
    if (current.kind !== "present" || current.fingerprint !== connection.tokenFingerprint) {
      throw new Error(current.kind === "unsafe" ? current.reason : "connected service token ownership changed");
    }
    writeTokenBackup(current.fingerprint);
    const rotation = await startClientKeyRotation(connection.managementUrl, authority, connection.apiKeyId, { fetchImpl: deps.fetchImpl });
    started = { rotationId: rotation.rotationId, key: rotation.key, createdAt: rotation.createdAt };
    const marked: OcxClientConnectionConfig = {
      ...connection,
      pendingOperation: {
        kind: "rotate",
        rotationId: rotation.rotationId,
        newKeyIssuedAt: rotation.createdAt,
        oldKeyBackupPath: serviceApiTokenBackupPath(),
      },
    };
    commitClientConnection(marked);
    connection = marked;
    markerPersisted = true;
    const replacement = replaceServiceApiTokenFile(rotation.key);
    if (!await probeClientKeyId(connection.serverUrl, rotation.key, connection.apiKeyId, { fetchImpl: deps.fetchImpl })) {
      throw new Error("new client key admission probe was refused");
    }
    try {
      await commitClientKeyRotation(connection.managementUrl, authority, connection.apiKeyId, rotation.rotationId, { fetchImpl: deps.fetchImpl });
    } catch {
      return await recoverRotationWithAuthority(connection, authority, deps);
    }
    const next = clearRotationState(connection, replacement.fingerprint);
    removeOrphanTokenBackup();
    return next;
  } catch (error) {
    if (error instanceof RotationRecoveryRequiredError) throw error;
    if (connection && authority && started) {
      if (markerPersisted && connection.pendingOperation) {
        try {
          const restored = restoreTokenBackup(connection.pendingOperation.oldKeyBackupPath);
          await abortClientKeyRotation(connection.managementUrl, authority, connection.apiKeyId, started.rotationId, { fetchImpl: deps.fetchImpl });
          clearRotationState(connection, restored.fingerprint);
          removeOrphanTokenBackup();
        } catch (recoveryError) {
          throw new RotationRecoveryRequiredError(
            "rotation rollback was incomplete; preserve service-api-token and .prev and rerun ocx connect rotate with transient authority",
            { cause: recoveryError },
          );
        }
      } else {
        try { await abortClientKeyRotation(connection.managementUrl, authority, connection.apiKeyId, started.rotationId, { fetchImpl: deps.fetchImpl }); }
        finally { removeOrphanTokenBackup(); }
      }
    } else {
      const backup = readTokenBackupState();
      if (backup.kind === "present") removeOrphanTokenBackup();
    }
    throw error;
  } finally {
    if (started) started.key = "";
    authority = null;
    releaseCredential(options.credential);
  }
}

async function cleanupIssuedKey(
  managementUrl: string,
  credential: { kind: "admin"; value: Uint8Array } | { kind: "gui-session"; value: ConnectGuiSession },
  issuedId: string,
  deps: ClientConnectDeps,
): Promise<string | null> {
  try {
    await revokeClientKey(managementUrl, credential, issuedId, { fetchImpl: deps.fetchImpl });
    return null;
  } catch {
    return `Hub cleanup could not revoke client key ${issuedId}; revoke it from Integrations → API Keys.`;
  }
}

export async function connectClient(
  options: ConnectOptions,
  deps: ClientConnectDeps = {},
): Promise<OcxClientConnectionConfig> {
  let serverUrl = "";
  let managementUrl = "";
  let issued: IssuedClientKey | null = null;
  let cleanupCredential: { kind: "admin"; value: Uint8Array } | { kind: "gui-session"; value: ConnectGuiSession } | null = null;
  let tokenFingerprint: string | null = null;
  let priorCatalog: CatalogSnapshot | null = null;
  let writtenCatalogFingerprint: string | null = null;
  let injectionCommitted = false;
  let committed = false;
  try {
    serverUrl = normalizeHubOrigin(options.serverUrl);
    if (options.managementUrl) managementUrl = normalizeHubOrigin(options.managementUrl);
    if (options.selectedClients.length < 1 || new Set(options.selectedClients).size !== options.selectedClients.length) {
      throw new Error("at least one unique connected client is required");
    }
    const state = readClientConnectionState();
    if (state.kind !== "disconnected") {
      const detail = state.kind === "connected" ? "already connected" : state.reason;
      throw new Error(`connect refused: client state is ${state.kind} (${detail})`);
    }
    const externalProvider = currentExternalCodexModelProvider();
    if (externalProvider) throw new Error(`connect refused: external Codex provider ${externalProvider} owns config.toml`);
    const tokenState = readServiceApiTokenState();
    if (tokenState.kind !== "absent") {
      throw new Error(tokenState.kind === "unsafe" ? tokenState.reason : "connect refused: service token file already exists");
    }

    const ready = await fetchHubReady(serverUrl, { fetchImpl: deps.fetchImpl });
    if (ready.status !== "ready") throw new Error(`hub is not ready (${ready.status})`);
    managementUrl = managementUrl || ready.metadata.managementUrl;
    if (options.managementTransport === "relay") {
      throw new Error("relay management transport is not available before Remote Hub Phase 4");
    }

    if (options.credential.kind === "pairing-grant") {
      const session = await exchangeConnectPairingGrant(
        managementUrl,
        localGuiOrigin(),
        options.credential.value,
        { allowInsecureHttp: options.allowInsecureHttp, fetchImpl: deps.fetchImpl },
      );
      cleanupCredential = { kind: "gui-session", value: session };
    } else {
      cleanupCredential = { kind: "admin", value: options.credential.value };
    }
    issued = await issueClientKey(managementUrl, cleanupCredential, clientKeyName(), { fetchImpl: deps.fetchImpl });

    priorCatalog = catalogSnapshot();
    const persisted = writeServiceApiTokenFile(issued.key);
    tokenFingerprint = persisted.fingerprint;

    const catalog = await downloadClientCatalog(serverUrl, issued.key, { fetchImpl: deps.fetchImpl });
    if (catalog.kind !== "fresh" || !catalog.etag) {
      throw new Error("initial hub catalog did not include a fresh ETag");
    }
    atomicWriteFile(DEFAULT_CATALOG_PATH, catalog.body);
    writtenCatalogFingerprint = sha256(catalog.body);

    const config = loadConfig();
    const target = routingTarget(serverUrl);
    const injectConfig = { ...config, syncResumeHistory: false };
    const preflight = await injectCodexConfig(config.port, injectConfig, {
      validateOnly: true,
      routingTarget: target,
      catalogPath: DEFAULT_CATALOG_PATH,
      journalOwner: { kind: "client", apiKeyId: issued.id },
    });
    if (!preflight.success) throw new Error(preflight.message);

    if (!options.noSync && options.selectedClients.includes("codex")) {
      const injected = await injectCodexConfig(config.port, injectConfig, {
        routingTarget: target,
        catalogPath: DEFAULT_CATALOG_PATH,
        journalOwner: { kind: "client", apiKeyId: issued.id },
      });
      if (!injected.success || injected.status === "skipped") throw new Error(injected.message);
      injectionCommitted = true;
      if (!isCodexRoutingInjected()) throw new Error("Codex routing target was not committed");
    }

    const now = (deps.now ?? (() => new Date()))().toISOString();
    const connection: OcxClientConnectionConfig = {
      serverUrl,
      managementUrl,
      managementTransport: options.managementTransport,
      selectedClients: [...options.selectedClients],
      tokenEnv: "OPENCODEX_API_AUTH_TOKEN",
      apiKeyId: issued.id,
      tokenFingerprint: persisted.fingerprint,
      protocolVersion: 1,
      connectedAt: now,
      catalogEtag: catalog.etag,
      catalogSyncedAt: now,
    };
    commitClientConnection(connection);
    committed = true;
    return connection;
  } catch (error) {
    const rollbackFailures: string[] = [];
    if (injectionCommitted) {
      const restored = restoreJournalState();
      if (!restored.complete) rollbackFailures.push("Codex journal restore was partial");
    }
    if (priorCatalog && writtenCatalogFingerprint && !restoreCatalogSnapshot(priorCatalog, writtenCatalogFingerprint)) {
      rollbackFailures.push("catalog rollback did not match the written artifact");
    }
    if (tokenFingerprint) {
      const removed = removeServiceApiTokenFileIfOwned(tokenFingerprint);
      if (removed === "changed") rollbackFailures.push("service token changed during rollback");
    }
    let remoteCleanup: string | null = null;
    if (issued && cleanupCredential && managementUrl) {
      remoteCleanup = await cleanupIssuedKey(managementUrl, cleanupCredential, issued.id, deps);
    }
    const base = error instanceof Error ? error.message : String(error);
    const details = [
      ...rollbackFailures,
      ...(remoteCleanup ? [remoteCleanup] : []),
    ];
    throw new Error(details.length > 0 ? `${base}. ${details.join(" ")}` : base, { cause: error });
  } finally {
    releaseCredential(options.credential);
    cleanupCredential = null;
    issued = null;
    if (!committed) {
      tokenFingerprint = null;
      priorCatalog = null;
      writtenCatalogFingerprint = null;
    }
  }
}

export async function syncConnectedClient(
  _options: { restartCodex?: boolean } = {},
  deps: ClientConnectDeps = {},
): Promise<{ catalogWritten: boolean; cacheSynced: boolean; injected: boolean; stale: boolean }> {
  const state = readClientConnectionState();
  if (state.kind !== "connected") throw new Error(`connected sync refused: client state is ${state.kind}`);
  const token = readServiceApiTokenState();
  if (token.kind !== "present" || token.fingerprint !== state.value.tokenFingerprint) {
    throw new Error(token.kind === "absent" ? "connected service token is missing" : "connected service token ownership changed");
  }

  let catalogWritten = false;
  let stale = false;
  let next = state.value;
  try {
    const downloaded = await downloadClientCatalog(state.value.serverUrl, token.token, {
      etag: state.value.catalogEtag,
      fetchImpl: deps.fetchImpl,
    });
    if (downloaded.kind === "fresh") {
      atomicWriteFile(DEFAULT_CATALOG_PATH, downloaded.body);
      catalogWritten = true;
      const now = (deps.now ?? (() => new Date()))().toISOString();
      next = {
        ...state.value,
        ...(downloaded.etag ? { catalogEtag: downloaded.etag } : {}),
        catalogSyncedAt: now,
      };
      commitClientConnection(next);
    } else {
      validLocalCatalog();
    }
  } catch (error) {
    const transient = error instanceof HubClientError
      && (error.code === "unreachable" || (error.status !== undefined && error.status >= 500));
    if (!transient) throw error;
    validLocalCatalog();
    stale = true;
  }

  let injected = false;
  if (next.selectedClients.includes("codex")) {
    const config = loadConfig();
    const result = await injectCodexConfig(config.port, { ...config, syncResumeHistory: false }, {
      routingTarget: routingTarget(next.serverUrl),
      catalogPath: DEFAULT_CATALOG_PATH,
      journalOwner: { kind: "client", apiKeyId: next.apiKeyId },
    });
    if (!result.success || result.status === "skipped") throw new Error(result.message);
    injected = true;
  }
  const cacheSynced = invalidateCodexModelsCache({ allowWhenDesiredDisabled: true });
  return { catalogWritten, cacheSynced, injected, stale };
}

function removeOwnedCatalog(connection: OcxClientConnectionConfig): "removed" | "absent" | "changed" {
  if (!existsSync(DEFAULT_CATALOG_PATH)) return "absent";
  try {
    const body = validLocalCatalog();
    if (!catalogMatchesEtag(body, connection.catalogEtag)) return "changed";
    unlinkSync(DEFAULT_CATALOG_PATH);
    return "removed";
  } catch {
    return "changed";
  }
}

export async function disconnectClient(
  options: { keepCatalog?: boolean } = {},
): Promise<{ restored: boolean; tokenRemoved: boolean; catalogRemoved: boolean; apiKeyId: string }> {
  const state = readClientConnectionState();
  if (state.kind !== "connected") throw new Error(`disconnect refused: client state is ${state.kind}`);
  const token = readServiceApiTokenState();
  if (token.kind !== "present" || token.fingerprint !== state.value.tokenFingerprint) {
    throw new Error(token.kind === "absent" ? "disconnect refused: service token is missing" : "disconnect refused: service token ownership changed");
  }

  let restored = true;
  if (state.value.selectedClients.includes("codex")) {
    const owner = journalOwner();
    if (owner?.kind === "client" && owner.apiKeyId === state.value.apiKeyId) {
      restored = restoreJournalState().complete;
    } else if (owner !== null || isCodexRoutingInjected()) {
      throw new Error("disconnect refused: Codex journal ownership conflicts with the connected key");
    }
    if (!restored) throw new Error("disconnect refused: Codex journal restore was partial");
  }

  const tokenRemoval = removeServiceApiTokenFileIfOwned(state.value.tokenFingerprint);
  if (tokenRemoval === "changed") throw new Error("disconnect refused: service token changed before removal");
  let catalogRemoval: "removed" | "absent" | "changed" = "absent";
  if (!options.keepCatalog) {
    catalogRemoval = removeOwnedCatalog(state.value);
    if (catalogRemoval === "changed") throw new Error("disconnect refused: catalog ownership changed");
  }
  if (clearClientConnection(state.value.apiKeyId) !== "committed") {
    throw new Error("disconnect refused: client state changed before final commit");
  }
  return {
    restored,
    tokenRemoved: tokenRemoval === "removed",
    catalogRemoved: catalogRemoval === "removed",
    apiKeyId: state.value.apiKeyId,
  };
}

export async function revokeConnectedClientKey(
  credential: { kind: "admin"; value: Uint8Array },
  deps: ClientConnectDeps = {},
): Promise<{ apiKeyId: string }> {
  try {
    const state = readClientConnectionState();
    if (state.kind !== "connected") throw new Error("connect revoke is available only while connected");
    await revokeClientKey(state.value.managementUrl, credential, state.value.apiKeyId, { fetchImpl: deps.fetchImpl });
    return { apiKeyId: state.value.apiKeyId };
  } finally {
    credential.value.fill(0);
  }
}
