export default function TrainingLoading() {
  return (
    <main className="training-page" aria-busy="true" aria-live="polite">
      <div className="training-shell">
        <div className="loading-panel">
          <span className="loading-dot" aria-hidden="true" />
          <p>Loading upcoming training sessions…</p>
        </div>
      </div>
    </main>
  );
}
