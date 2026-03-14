"use client";

import { useEffect } from "react";

export default function Error({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    console.error("[ARGUS] Unhandled error:", error);
  }, [error]);

  return (
    <div className="min-h-screen flex items-center justify-center bg-black text-white p-8">
      <div className="max-w-md text-center space-y-6">
        <div className="text-4xl font-mono font-bold tracking-tight">ARGUS</div>
        <p className="text-white/60 text-sm">Something went wrong.</p>
        <p className="text-white/40 text-xs font-mono break-all">
          {error.message || "An unexpected error occurred."}
        </p>
        <button
          type="button"
          onClick={reset}
          className="px-6 py-2 rounded-full bg-white/10 hover:bg-white/20 text-sm font-medium transition-colors"
        >
          Try again
        </button>
      </div>
    </div>
  );
}
