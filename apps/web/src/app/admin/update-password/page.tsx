"use client";

import Link from "next/link";
import { FormEvent, useEffect, useState } from "react";
import { getSupabaseBrowserClient } from "@/lib/supabase";
import ClubMobileMenu from "@/components/club-mobile-menu";

const supabase = getSupabaseBrowserClient();

export default function UpdatePasswordPage() {
  const [recoveryReady, setRecoveryReady] = useState(false);
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [success, setSuccess] = useState(false);

  useEffect(() => {
    if (!supabase) {
      setError("Password recovery is not configured. Contact the site administrator.");
      return;
    }

    const callbackUrl = new URL(window.location.href);
    const callbackParams = new URLSearchParams(callbackUrl.hash.replace(/^#/, ""));
    const authError = callbackUrl.searchParams.get("error_description")
      || callbackUrl.searchParams.get("error")
      || callbackParams.get("error_description")
      || callbackParams.get("error");
    const hasRecoveryCallback = callbackUrl.searchParams.has("code") || callbackParams.get("type") === "recovery";

    if (authError) {
      setError("This password recovery link is invalid or has expired. Request a new link from admin sign-in.");
      return;
    }
    if (!hasRecoveryCallback) {
      setError("Open this page from the password recovery email. If the link has expired, request a new one from admin sign-in.");
      return;
    }

    const recoveryTimeout = window.setTimeout(() => {
      setError("Supabase could not establish a recovery session. The link may be invalid or expired, or Auth may be temporarily unavailable. Request a new link and try again.");
    }, 10000);
    const { data: { subscription } } = supabase.auth.onAuthStateChange((event, session) => {
      if (event === "PASSWORD_RECOVERY" && session) {
        window.clearTimeout(recoveryTimeout);
        setRecoveryReady(true);
        setError("");
      }
    });

    return () => {
      window.clearTimeout(recoveryTimeout);
      subscription.unsubscribe();
    };
  }, []);

  async function updatePassword(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    if (!recoveryReady || !supabase) {
      setError("The recovery session is no longer valid. Request a new recovery link.");
      return;
    }
    if (password.length < 8) {
      setError("Use a password with at least 8 characters.");
      return;
    }
    if (password !== confirmation) {
      setError("The passwords do not match.");
      return;
    }

    setBusy(true);
    try {
      const { error: updateError } = await supabase.auth.updateUser({ password });
      if (updateError) throw new Error("The password could not be updated. The recovery link may have expired; request a new one and try again.");
      await supabase.auth.signOut({ scope: "local" }).catch(() => undefined);
      setSuccess(true);
      setPassword("");
      setConfirmation("");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "The password could not be updated.");
    } finally { setBusy(false); }
  }

  return (
    <main className="admin-page">
      <header className="training-header"><Link className="brand" href="/" aria-label="MMSU Tennis Club home"><span className="brand-mark" aria-hidden="true">M</span><span><strong>MMSU</strong><small>TENNIS CLUB</small></span></Link><span className="header-note">Admin account</span><ClubMobileMenu /></header>
      <section className="admin-main">
        <div className="admin-login">
          <p className="section-kicker">PASSWORD RECOVERY</p>
          <h1>Choose a new password.</h1>
          {success ? <>
            <p className="form-success" role="status">Your password has been updated. Return to admin sign-in and use your new password.</p>
            <div className="admin-actions"><Link className="admin-button" href="/admin">Back to admin sign-in</Link></div>
          </> : recoveryReady ? <form onSubmit={updatePassword}>
            <label>New password<input type="password" value={password} onChange={event => setPassword(event.target.value)} autoComplete="new-password" minLength={8} required /></label>
            <label>Confirm new password<input type="password" value={confirmation} onChange={event => setConfirmation(event.target.value)} autoComplete="new-password" minLength={8} required /></label>
            {error && <p className="form-error" role="alert">{error}</p>}
            <p className="admin-help">Use at least 8 characters. Your password is sent directly to Supabase Auth over HTTPS.</p>
            <button className="admin-button" disabled={busy} type="submit">{busy ? "Updating password…" : "Update password"}</button>
          </form> : <>
            {error && <p className="form-error" role="alert">{error}</p>}
            <p className="admin-help">The recovery link is checked by Supabase Auth before this form is enabled.</p>
          </>}
          {!success && <div className="admin-actions"><Link className="admin-button secondary" href="/admin">Back to admin sign-in</Link></div>}
        </div>
      </section>
    </main>
  );
}
