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

  const verify = async (e) => {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      const data = await api(apiBaseUrl, "/auth/verify", { method: "POST", json: { email: email.trim(), code: code.trim() } });
      onLoginSuccess(data.token, data.agent_id, data.user_id, email.trim());
    } catch (err) {
      setError(err.status ? err.message : "Couldn't reach the Memro API.");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-full flex flex-col items-center justify-center p-4">
      <div className="w-full max-w-[380px]">
        <div className="flex items-center gap-2.5 mb-8 justify-center">
          <Wordmark className="h-8" />
        </div>

        <div className="card bg-panel overflow-hidden">
          <div className="px-5 h-11 flex items-center">
            <p className="label">{step === "email" ? "Sign in" : "Check your email"}</p>
          </div>
          <div className="bg-surface border-t border-line rounded-xl -mx-px -mb-px p-5">
            {step === "email" ? (
              <form onSubmit={sendCode} className="space-y-4">
                <div>
                  <h1 className="text-[18px] font-semibold tracking-tight">Welcome to Memro</h1>
                  <p className="text-[13px] text-muted mt-1">Enter your email and we'll send you a sign-in code.</p>
                </div>
                <input
                  type="email"
                  autoFocus
                  required
                  autoComplete="email"
                  className="input h-10"
                  placeholder="you@company.com"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                />
                {error && <p className="text-[12px] text-danger">{error}</p>}
                <button className="btn-primary w-full h-10" disabled={loading}>{loading ? "Sending…" : "Continue with email"}</button>
              </form>
            ) : (
              <form onSubmit={verify} className="space-y-4">
                <div>
                  <h1 className="text-[18px] font-semibold tracking-tight">Enter your code</h1>
                  <p className="text-[13px] text-muted mt-1">We sent a 6-digit code to <span className="text-ink">{email}</span>. It expires in 15 minutes.</p>
                </div>
                <input
                  autoFocus
                  required
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  maxLength={6}
                  className="input h-12 text-center font-mono text-[22px] tracking-[0.5em]"
                  placeholder="000000"
                  value={code}
                  onChange={(e) => setCode(e.target.value.replace(/\D/g, ""))}
                />
                {error && <p className="text-[12px] text-danger">{error}</p>}
                <button className="btn-primary w-full h-10" disabled={loading || code.length < 6}>{loading ? "Verifying…" : "Sign in"}</button>
                <button type="button" className="btn-ghost w-full" onClick={() => { setStep("email"); setCode(""); setError(""); }}>
                  <Icon name="arrow_back" className="!text-[16px]" /> Use a different email
                </button>
              </form>
            )}
          </div>
        </div>

        <p className="text-center text-[12px] text-subtle mt-6">
          Memro Cloud ·{" "}
          <a className="hover:text-ink underline-offset-2 hover:underline" href="https://memro.co/terms.html">Terms</a> ·{" "}
          <a className="hover:text-ink underline-offset-2 hover:underline" href="https://memro.co/privacy.html">Privacy</a>
        </p>
      </div>
    </div>
  );
}
