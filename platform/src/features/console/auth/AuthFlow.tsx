"use client";

import { useState, type FormEvent } from "react";
import Link from "next/link";

import styles from "./auth.module.css";

type Step = "identify" | "verify" | "done";

type AuthFlowProps = {
  initialUserCode?: string;
  mode: "activate" | "login";
};

export function AuthFlow({ initialUserCode = "", mode }: AuthFlowProps) {
  const [step, setStep] = useState<Step>("identify");
  const [email, setEmail] = useState("");
  const [userCode, setUserCode] = useState(initialUserCode);
  const [otp, setOtp] = useState("");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");

  async function requestCode(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setMessage("");
    try {
      let code = userCode;
      if (mode === "login") {
        const authorization = await postJson<{
          userCode: string;
        }>("/api/console/auth/device-authorizations", {
          clientName: "Web console",
          clientType: "browser",
        });
        code = authorization.userCode;
        setUserCode(code);
      }
      await postJson("/api/console/auth/verification-emails", { email, userCode: code });
      setStep("verify");
      setMessage("If this account is allowed, a six-digit code is on its way.");
    } catch (error) {
      setMessage(messageFor(error));
    } finally {
      setBusy(false);
    }
  }

  async function approve(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setMessage("");
    try {
      const result = await postJson<{ browserSession: boolean }>("/api/console/auth/decisions", {
        decision: "approve",
        email,
        otp,
        userCode,
      });
      setStep("done");
      setOtp("");
      setEmail("");
      setUserCode("");
      if (result.browserSession) {
        window.location.assign("/console");
        return;
      }
      setBusy(false);
      setMessage("Device approved. Return to the device to finish signing in; you can close this page.");
    } catch (error) {
      setMessage(messageFor(error));
      setBusy(false);
    }
  }

  async function deny() {
    if (!/^\d{6}$/u.test(otp)) {
      setMessage("Enter the six-digit email code before denying this request.");
      return;
    }
    setBusy(true);
    setMessage("");
    try {
      await postJson("/api/console/auth/decisions", {
        decision: "deny",
        email,
        otp,
        userCode,
      });
      setStep("done");
      setBusy(false);
      setOtp("");
      setEmail("");
      setUserCode("");
      setMessage("Device access was denied. You can close this page.");
    } catch (error) {
      setMessage(messageFor(error));
      setBusy(false);
    }
  }

  return (
    <main className={styles.page}>
      <Link className={styles.brand} href="/">f· file.cheap</Link>
      <section className={styles.card} aria-labelledby="auth-title">
        <p className={styles.eyebrow}>{mode === "login" ? "Private console" : "Device pairing"}</p>
        <h1 id="auth-title">{mode === "login" ? "Open your artifact vault" : "Approve a device"}</h1>
        <p className={styles.intro}>
          {mode === "login"
            ? "We will email a one-time code. No reusable browser token is exposed to JavaScript."
            : "Match the code shown by your CLI, verify the owner email, then explicitly approve the request."}
        </p>

        {step === "identify" ? (
          <form className={styles.form} onSubmit={requestCode}>
            {mode === "activate" ? (
              <label>Pairing code
                <input autoComplete="one-time-code" maxLength={9} onChange={(event) => setUserCode(event.target.value.toUpperCase())} placeholder="F7KQ-8P2M" required value={userCode} />
              </label>
            ) : null}
            <label>Owner email
              <input autoComplete="email" onChange={(event) => setEmail(event.target.value)} required type="email" value={email} />
            </label>
            <button disabled={busy} type="submit">{busy ? "Requesting…" : "Email verification code"}</button>
          </form>
        ) : step === "verify" ? (
          <form className={styles.form} onSubmit={approve}>
            <div className={styles.deviceReceipt}>
              <span>Pairing code</span><strong>{userCode}</strong>
              <span>Email</span><strong>{email}</strong>
            </div>
            <label>Six-digit email code
              <input autoComplete="one-time-code" inputMode="numeric" maxLength={6} onChange={(event) => setOtp(event.target.value.replace(/\D/gu, ""))} pattern="[0-9]{6}" required value={otp} />
            </label>
            <button disabled={busy} type="submit">{busy ? "Approving…" : mode === "login" ? "Approve and continue" : "Approve device"}</button>
            <button className={styles.secondary} disabled={busy} onClick={deny} type="button">{mode === "login" ? "Cancel login" : "Deny device"}</button>
            <button className={styles.secondary} disabled={busy} onClick={() => { setStep("identify"); setBusy(false); setOtp(""); }} type="button">Start over</button>
          </form>
        ) : (
          <div aria-live="polite" className={styles.done}>
            <span aria-hidden="true">✓</span>
            <h2>Request complete</h2>
            <p>{message}</p>
          </div>
        )}
        {step !== "done" ? <p aria-live="polite" className={styles.message}>{message}</p> : null}
        <p className={styles.boundary}>Opening an email never approves a device. Codes expire after ten minutes and can be used once.</p>
      </section>
    </main>
  );
}

async function postJson<T = unknown>(url: string, body: unknown): Promise<T> {
  const response = await fetch(url, {
    body: JSON.stringify(body),
    headers: { "content-type": "application/json" },
    method: "POST",
  });
  const payload = await response.json() as { detail?: string } & T;
  if (!response.ok) throw new Error(payload.detail ?? "The request could not be completed.");
  return payload;
}

function messageFor(error: unknown): string {
  return error instanceof Error ? error.message : "The request could not be completed.";
}
