import Link from "next/link";

export default function Home() {
  return (
    <div className="relative min-h-screen flex items-center justify-center overflow-hidden">
      {/* Dynamic Background Elements */}
      <div className="absolute inset-0 z-0">
        <div className="absolute top-1/4 left-1/4 w-96 h-96 bg-blue-500/20 rounded-full mix-blend-multiply filter blur-3xl opacity-70 animate-blob" />
        <div className="absolute top-1/3 right-1/4 w-96 h-96 bg-purple-500/20 rounded-full mix-blend-multiply filter blur-3xl opacity-70 animate-blob animation-delay-2000" />
        <div className="absolute bottom-1/4 left-1/3 w-96 h-96 bg-pink-500/20 rounded-full mix-blend-multiply filter blur-3xl opacity-70 animate-blob animation-delay-4000" />
      </div>

      {/* Main Content */}
      <main className="z-10 flex flex-col items-center justify-center w-full max-w-5xl px-6 text-center">
        <div className="glass-card rounded-3xl p-12 md:p-16 flex flex-col items-center max-w-2xl w-full border-t border-b-0 border-r-0 border-l border-white/40 shadow-2xl relative overflow-hidden">
          {/* Subtle glow effect on card */}
          <div className="absolute top-0 left-0 w-full h-1 bg-gradient-to-r from-transparent via-white/50 to-transparent"></div>

          <div className="mb-8 p-4 rounded-2xl bg-white/5 dark:bg-black/20 border border-white/10 shadow-inner">
            <svg
              className="w-16 h-16 text-zinc-800 dark:text-zinc-200"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
              xmlns="http://www.w3.org/2000/svg"
            >
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
            </svg>
          </div>

          <h1 className="text-4xl md:text-5xl font-extrabold tracking-tight text-zinc-900 dark:text-white mb-6">
            Kaplabs Auth
          </h1>
          <p className="text-lg md:text-xl text-zinc-600 dark:text-zinc-300 mb-10 max-w-lg leading-relaxed">
            One secure identity provider for all your connected applications. Single Sign-On simplified.
          </p>

          <div className="flex flex-col sm:flex-row gap-4 w-full sm:w-auto mt-4">
            <Link
              href="/admin"
              className="px-8 py-3 rounded-full bg-zinc-900 dark:bg-white text-white dark:text-black font-medium hover:scale-105 transition-transform duration-300 shadow-lg"
            >
              Admin Portal Login
            </Link>
          </div>
        </div>
      </main>
    </div>
  );
}
