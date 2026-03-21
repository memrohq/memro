import { useState } from "react";

const Login = ({ apiBaseUrl, onLoginSuccess }) => {
  const [step, setStep] = useState('email');
  const [email, setEmail] = useState('');
  const [otp, setOtp] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [message, setMessage] = useState('');

  const handleEmailSubmit = async (e) => {
    e.preventDefault();
    if (!email) return;
    setError('');
    setMessage('');
    setLoading(true);
    try {
      const res = await fetch(`${apiBaseUrl}/auth/login`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email })
      });
      if (res.ok) {
        setStep('otp');
        setMessage('OTP sent! Check your developer console/backend logs.');
      } else {
        const text = await res.text();
        setError(text || "Failed to send OTP.");
      }
    } catch (e) {
      setError('Network error: Could not reach backend.');
    } finally {
      setLoading(false);
    }
  };

  const handleOtpSubmit = async (e) => {
    e.preventDefault();
    if (!otp) return;
    setError('');
    setLoading(true);
    try {
      const res = await fetch(`${apiBaseUrl}/auth/verify`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email, code: otp })
      });
      if (res.ok) {
        const data = await res.json();
        onLoginSuccess(data.token, data.agent_id, data.private_key, data.user_id, email);
      } else {
        const text = await res.text();
        setError(text || "Invalid OTP.");
      }
    } catch (e) {
      setError('Network error: Could not reach backend.');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-slate-50 font-display">
      <div className="max-w-md w-full p-8 bg-white border border-primary/20 shadow-xl relative overflow-hidden">
        {/* Background Blueprint Grid */}
        <div className="absolute inset-0 pointer-events-none opacity-[0.03]" 
             style={{ backgroundImage: 'linear-gradient(#0f172a 1px, transparent 1px), linear-gradient(90deg, #0f172a 1px, transparent 1px)', backgroundSize: '20px 20px' }} />
        
        <div className="relative z-10 flex flex-col items-center mb-8">
          <svg className="size-12 text-primary mb-4" fill="none" viewBox="0 0 48 48" xmlns="http://www.w3.org/2000/svg">
            <path d="M36.7273 44C33.9891 44 31.6043 39.8386 30.3636 33.69C29.123 39.8386 26.7382 44 24 44C21.2618 44 18.877 39.8386 17.6364 33.69C16.3957 39.8386 14.0109 44 11.2727 44C7.25611 44 4 35.0457 4 24C4 12.9543 7.25611 4 11.2727 4C14.0109 4 16.3957 8.16144 17.6364 14.31C18.877 8.16144 21.2618 4 24 4C26.7382 4 29.123 8.16144 30.3636 14.31C31.6043 8.16144 33.9891 4 36.7273 4C40.7439 4 44 12.9543 44 24C44 35.0457 40.7439 44 36.7273 44Z" fill="currentColor"></path>
          </svg>
          <h1 className="text-2xl font-black text-primary uppercase tracking-widest text-center">MEMRO.CO</h1>
          <p className="text-xs font-mono text-slate-500 tracking-tight mt-1">Multi-Tenant Cloud Access</p>
        </div>

        {error && <div className="mb-4 p-3 bg-red-50 border border-red-200 text-red-600 text-xs font-mono text-center">{error}</div>}
        {message && <div className="mb-4 p-3 bg-green-50 border border-green-200 text-green-700 text-xs font-mono text-center">{message}</div>}

        {step === 'email' ? (
          <form onSubmit={handleEmailSubmit} className="space-y-4">
            <div>
              <label className="block text-xs font-bold text-primary uppercase tracking-wider mb-2">Email Address</label>
              <input
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                className="w-full border-2 border-primary/20 focus:border-primary p-3 bg-slate-50 outline-none text-sm transition-colors"
                placeholder="developer@startup.com"
                required
              />
            </div>
            <button
              type="submit"
              disabled={loading}
              className="w-full bg-primary text-white py-3 font-bold uppercase text-xs tracking-widest hover:bg-primary/90 transition-colors disabled:opacity-50"
            >
              {loading ? 'SENDING...' : 'CONTINUE WITH EMAIL'}
            </button>
          </form>
        ) : (
          <form onSubmit={handleOtpSubmit} className="space-y-4">
            <div>
              <label className="block text-xs font-bold text-primary uppercase tracking-wider mb-2">One-Time Passcode</label>
              <input
                type="text"
                value={otp}
                onChange={(e) => setOtp(e.target.value)}
                className="w-full border-2 border-primary/20 focus:border-primary p-4 text-center tracking-[0.6em] text-xl font-mono bg-slate-50 outline-none transition-colors"
                placeholder="000000"
                maxLength={8}
                required
              />
            </div>
            <button
              type="submit"
              disabled={loading}
              className="w-full bg-primary text-white py-3 font-bold uppercase text-xs tracking-widest hover:bg-primary/90 transition-colors disabled:opacity-50"
            >
              {loading ? 'VERIFYING...' : 'SECURE LOGIN'}
            </button>
            <button
              type="button"
              onClick={() => setStep('email')}
              className="w-full border-2 border-primary/20 text-primary py-3 font-bold uppercase text-xs tracking-widest hover:bg-primary/5 transition-colors"
            >
              USE DIFFERENT EMAIL
            </button>
          </form>
        )}
      </div>
    </div>
  );
};

export default Login;
