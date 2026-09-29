import { RefreshCw } from "lucide-react";

import { SessionRows, useSessions } from "@/features/sessions/sessions-page";

export function RecentSessions({ appId, sessionId }: { appId: string; sessionId: string }) {
  const { items, loading, error, cursor, refresh, loadMore } = useSessions(appId);
  return (
    <aside aria-label="Recent sessions" className="session-panel session-recent">
      <div className="session-panel-heading">
        <div>
          <h2>Recent sessions</h2>
          <p>Only session-tagged traces appear here</p>
        </div>
        <button
          aria-label="Refresh sessions"
          className="session-icon-button"
          disabled={loading}
          onClick={refresh}
          type="button"
        >
          <RefreshCw aria-hidden="true" size={14} />
        </button>
      </div>
      {loading && items.length === 0 && (
        <p className="session-panel-note" role="status">
          Loading sessions…
        </p>
      )}
      {error !== null && (
        <p className="session-panel-note text-danger" role="alert">
          {error}
        </p>
      )}
      {!loading && error === null && items.length === 0 && (
        <p className="session-panel-note">No recent sessions. Your open session is still shown.</p>
      )}
      <SessionRows appId={appId} items={items} selectedId={sessionId} />
      {cursor !== null && (
        <div className="session-panel-note">
          <button
            className="session-control"
            disabled={loading}
            onClick={() => void loadMore()}
            type="button"
          >
            Load more sessions
          </button>
        </div>
      )}
    </aside>
  );
}
