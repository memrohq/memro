import { useState } from "react";
import { Icon, Wordmark } from "./ui";
import { api } from "../lib/api";

export default function Login({ apiBaseUrl, onLoginSuccess }) {
  const [step, setStep] = useState("email");
  const [email, setEmail] = useState("");
  const [code, setCode] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const sendCode = async (e) => {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      await api(apiBaseUrl, "/auth/login", { method: "POST", json: { email: email.trim() } });
      setStep("code");
    } catch (err) {
      setError(err.status ? err.message : "Couldn't reach the Memro API.");
    } finally {
      setLoading(false);
    }
  };

  const verify = async (e, value = code) => {
    e?.preventDefault();
    if (value.length < 6) return;
    setError("");
    setLoading(true);
    try {
      const data = await api(apiBaseUrl, "/auth/verify", { method: "POST", json: { email: email.trim(), code: value } });
      onLoginSuccess(data.token, data.agent_id, data.user_id, email.trim());
    } catch (err) {
      setError(err.status ? err.message : "Couldn't reach the Memro API.");
      setCode("");
    } finally {
      setLoading(false);
    }
  };

  const onCode = (v) => {
    const digits = v.replace(/\D/g, "").slice(0, 6);
    setCode(digits);
    if (digits.length === 6) verify(null, digits);
  };

  return (
    <div className="min-h-full grid lg:grid-cols-[minmax(0,1fr)_minmax(0,1.05fr)] bg-canvas">
      <div className="flex flex-col px-6 sm:px-12 py-8">
        <Wordmark className="h-7" />
        <div className="flex-1 flex items-center justify-center py-12">
          <div className="w-full max-w-[380px] pop-in">
            {step === "email" ? (
              <form onSubmit={sendCode} className="space-y-6">
                <div>
                  <h1 className="text-[28px] leading-tight font-semibold tracking-[-0.03em]">Sign in to Memro</h1>
                  <p className="text-[14px] text-muted mt-2">Long-term memory for your AI tools and agents. We'll email you a 6-digit code; no password needed.</p>
                </div>
                <label className="block">
                  <span className="block text-[12.5px] font-medium mb-1.5">Work email</span>
                  <input
                    type="email" autoFocus required autoComplete="email"
                    className="input h-11 text-[14px]" placeholder="you@company.com"
                    value={email} onChange={(e) => setEmail(e.target.value)}
                  />
                </label>
                {error && <p className="text-[13px] text-danger flex items-center gap-1.5"><Icon name="error" fill className="!text-[16px]" />{error}</p>}
                <button className="btn-primary w-full h-11 text-[14px]" disabled={loading}>
                  {loading ? <span className="size-4 rounded-full border-2 border-on-accent/30 border-t-on-accent animate-spin" /> : null}
                  {loading ? "Sending code…" : "Continue with email"}
                </button>
              </form>
            ) : (
              <form onSubmit={verify} className="space-y-6">
                <div>
                  <span className="size-11 rounded-2xl bg-accent-soft border border-accent-line flex items-center justify-center mb-5">
                    <Icon name="mark_email_unread" className="text-accent !text-[22px]" />
                  </span>
                  <h1 className="text-[28px] leading-tight font-semibold tracking-[-0.03em]">Check your email</h1>
                  <p className="text-[14px] text-muted mt-2">Enter the code we sent to <span className="text-ink font-medium">{email}</span>. It expires in 15 minutes.</p>
                </div>
                <label className="relative block" aria-label="6-digit code">
                  <input
                    autoFocus required inputMode="numeric" autoComplete="one-time-code" maxLength={6}
                    className="absolute inset-0 w-full h-full opacity-0 cursor-text"
                    value={code} onChange={(e) => onCode(e.target.value)} disabled={loading}
                  />
                  <span className="grid grid-cols-6 gap-2" aria-hidden="true">
                    {Array.from({ length: 6 }, (_, i) => {
                      const current = i === code.length && !loading;
                      return (
                        <span key={i} className={`h-14 rounded-xl border bg-surface flex items-center justify-center text-[24px] font-semibold tabular transition-[border-color,box-shadow] ${
                          current ? "border-accent ring-4 ring-accent/15" : code[i] ? "border-line-strong" : "border-line"}`}>
                          {code[i] || (current ? <span className="w-px h-6 bg-accent animate-pulse" /> : "")}
                        </span>
                      );
                    })}
                  </span>
                </label>
                {error && <p className="text-[13px] text-danger flex items-center gap-1.5"><Icon name="error" fill className="!text-[16px]" />{error}</p>}
                <button className="btn-primary w-full h-11 text-[14px]" disabled={loading || code.length < 6}>
                  {loading ? "Signing in…" : "Sign in"}
                </button>
                <div className="flex items-center justify-between text-[13px]">
                  <button type="button" className="text-muted hover:text-ink inline-flex items-center gap-1" onClick={() => { setStep("email"); setCode(""); setError(""); }}>
                    <Icon name="arrow_back" className="!text-[16px]" /> Different email
                  </button>
                  <button type="button" className="text-accent hover:text-accent-strong font-medium" onClick={sendCode} disabled={loading}>Resend code</button>
                </div>
              </form>
            )}
          </div>
        </div>
        <p className="text-[12px] text-subtle">
          © {new Date().getFullYear()} Memro ·{" "}
          <a className="hover:text-ink underline-offset-2 hover:underline" href="https://memro.co/terms.html">Terms</a> ·{" "}
          <a className="hover:text-ink underline-offset-2 hover:underline" href="https://memro.co/privacy.html">Privacy</a>
        </p>
      </div>

      <div className="hidden lg:block p-3">
        <div className="relative h-full rounded-[28px] overflow-hidden bg-[#0d0b1f] text-white">
          <div className="absolute inset-0 bg-[radial-gradient(60%_50%_at_20%_15%,rgba(123,148,236,0.55),transparent_70%),radial-gradient(50%_45%_at_85%_80%,rgba(122,178,174,0.45),transparent_70%),radial-gradient(45%_40%_at_70%_30%,rgba(87,70,213,0.6),transparent_70%)]" />
          <div className="absolute inset-0 opacity-[0.07] bg-[linear-gradient(rgba(255,255,255,1)_1px,transparent_1px),linear-gradient(90deg,rgba(255,255,255,1)_1px,transparent_1px)] bg-[size:40px_40px]" />
          <div className="relative h-full flex flex-col justify-between p-12">
            <img src="/memro-mark.png" alt="" className="size-12" />
            <div className="max-w-[460px]">
              <p className="text-[34px] leading-[1.15] font-semibold tracking-[-0.03em]">Your AI tools forget everything between sessions. Memro doesn't.</p>
              <ul className="mt-8 space-y-3.5 text-[14.5px] text-white/80">
                {[
                  ["cable", "One command connects Claude Code, Cursor, Codex and more"],
                  ["description", "Documents become searchable memories"],
                  ["lock", "Your memories are yours: private, exportable, deletable"],
                ].map(([icon, text]) => (
                  <li key={icon} className="flex items-center gap-3">
                    <span className="size-8 rounded-[10px] bg-white/10 border border-white/15 flex items-center justify-center">
                      <Icon name={icon} className="!text-[17px] text-white" />
                    </span>
                    {text}
                  </li>
                ))}
              </ul>
            </div>
            <p className="text-[12.5px] text-white/50">memro.co</p>
          </div>
        </div>
      </div>
    </div>
  );
}
