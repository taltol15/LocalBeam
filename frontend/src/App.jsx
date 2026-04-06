import { useState, useEffect, useCallback } from 'react';
import './App.css';
import { EventsOn } from '../wailsjs/runtime/runtime';
import { SelectFile, SendFile, GetMyPIN, ProtocolInfo } from '../wailsjs/go/main/App';

function peerAddress(peer) {
  const port = peer.port ?? 34567;
  return port === 34567 ? peer.ip : `${peer.ip}:${port}`;
}

function App() {
  const [peers, setPeers] = useState([]);
  const [myPin, setMyPin] = useState('····');
  const [protocolVer, setProtocolVer] = useState('');
  const [notifications, setNotifications] = useState([]);

  const [uploadProgress, setUploadProgress] = useState(0);
  const [isUploading, setIsUploading] = useState(false);

  const [downloadProgress, setDownloadProgress] = useState(0);
  const [isDownloading, setIsDownloading] = useState(false);

  const [manualHost, setManualHost] = useState('');

  const [pinModal, setPinModal] = useState(null);
  const [pinInput, setPinInput] = useState('');

  const addNotification = useCallback((notif) => {
    const id = Date.now();
    setNotifications((prev) => [...prev, { ...notif, id }]);
    setTimeout(() => {
      setNotifications((prev) => prev.filter((n) => n.id !== id));
    }, 5200);
  }, []);

  useEffect(() => {
    GetMyPIN().then(setMyPin);
    ProtocolInfo().then(setProtocolVer).catch(() => {});

    EventsOn('peer-found', (peer) => {
      setPeers((list) => {
        const addr = peerAddress(peer);
        if (list.some((p) => peerAddress(p) === addr)) return list;
        return [...list, peer];
      });
    });

    EventsOn('file-received', (info) => {
      setIsDownloading(false);
      setDownloadProgress(0);
      addNotification({
        type: 'success',
        title: 'File received',
        message: info.filename,
        sub: `From ${info.sender}\n${info.size}`,
      });
    });

    EventsOn('upload-progress', (percent) => {
      setUploadProgress(percent);
    });

    EventsOn('download-progress', (percent) => {
      if (percent < 100) setIsDownloading(true);
      setDownloadProgress(percent);
      if (percent >= 100) setIsDownloading(false);
    });
  }, [addNotification]);

  const copyPin = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(myPin);
      addNotification({ type: 'info', title: 'Copied', message: 'PIN copied to clipboard.' });
    } catch {
      addNotification({ type: 'error', title: 'Copy failed', message: 'Could not access clipboard.' });
    }
  }, [myPin, addNotification]);

  const beginSend = useCallback(async (address, displayName) => {
    const filePath = await SelectFile();
    if (!filePath) return;
    setPinInput('');
    setPinModal({ address, displayName, filePath });
  }, []);

  const confirmSendWithPin = useCallback(async () => {
    if (!pinModal) return;
    const pin = pinInput.trim();
    if (!/^\d{4}$/.test(pin)) {
      addNotification({ type: 'error', title: 'Invalid PIN', message: 'Enter the 4-digit code shown on the receiver.' });
      return;
    }
    const { address, displayName, filePath } = pinModal;
    setPinModal(null);

    setIsUploading(true);
    setUploadProgress(0);
    const result = await SendFile(address, filePath, pin);
    setIsUploading(false);
    setUploadProgress(0);

    if (result === 'Success') {
      addNotification({ type: 'success', title: 'Sent', message: `Delivered to ${displayName}.` });
    } else {
      addNotification({ type: 'error', title: 'Send failed', message: result.replace(/^Error:\s*/, '') });
    }
  }, [pinModal, pinInput, addNotification]);

  return (
    <div className="app">
      <div className="bg-grid" aria-hidden="true" />

      <header className="top">
        <div className="brand">
          <div className="brand-mark" aria-hidden="true" />
          <div>
            <h1>LocalBeam</h1>
            <p className="tagline">LAN file transfer · no cloud</p>
          </div>
        </div>

        <div className="pin-panel">
          <div className="pin-panel-label">
            <span>Your receive PIN</span>
            {protocolVer ? <span className="ver-pill">v{protocolVer}</span> : null}
          </div>
          <div className="pin-row">
            <code className="pin-value">{myPin}</code>
            <button type="button" className="btn ghost" onClick={copyPin}>
              Copy
            </button>
          </div>
          <p className="pin-hint">Share this code only with people you trust on your network.</p>
        </div>
      </header>

      {isUploading && (
        <div className="modal-backdrop" role="dialog" aria-label="Upload progress">
          <div className="modal sheet">
            <h2>Sending</h2>
            <div className="progress-track">
              <div className="progress-fill send" style={{ width: `${uploadProgress}%` }} />
            </div>
            <p className="progress-label">{uploadProgress}%</p>
          </div>
        </div>
      )}

      {isDownloading && (
        <div className="modal-backdrop" role="dialog" aria-label="Download progress">
          <div className="modal sheet">
            <h2>Receiving</h2>
            <div className="progress-track">
              <div className="progress-fill recv" style={{ width: `${downloadProgress}%` }} />
            </div>
            <p className="progress-label">{downloadProgress}%</p>
          </div>
        </div>
      )}

      {pinModal && (
        <div className="modal-backdrop" role="dialog" aria-modal="true" aria-labelledby="pin-title">
          <div className="modal sheet pin-sheet">
            <h2 id="pin-title">Receiver PIN</h2>
            <p className="modal-sub">Enter the PIN shown on {pinModal.displayName}.</p>
            <input
              className="pin-field"
              type="text"
              inputMode="numeric"
              maxLength={4}
              placeholder="0000"
              autoFocus
              value={pinInput}
              onChange={(e) => setPinInput(e.target.value.replace(/\D/g, '').slice(0, 4))}
              onKeyDown={(e) => e.key === 'Enter' && confirmSendWithPin()}
            />
            <div className="modal-actions">
              <button type="button" className="btn ghost" onClick={() => setPinModal(null)}>
                Cancel
              </button>
              <button type="button" className="btn primary" onClick={confirmSendWithPin}>
                Send
              </button>
            </div>
          </div>
        </div>
      )}

      <div className="toasts">
        {notifications.map((n) => (
          <div key={n.id} className={`toast ${n.type}`} role="status">
            <strong>{n.title}</strong>
            <div>{n.message}</div>
            {n.sub && <div className="toast-sub">{n.sub}</div>}
          </div>
        ))}
      </div>

      <section className="manual">
        <h2>Send by address</h2>
        <p className="section-desc">If a device does not appear, enter its IP (optional <code>:port</code>).</p>
        <div className="manual-row">
          <input
            className="text-input"
            placeholder="e.g. 192.168.1.24 or 192.168.1.24:34567"
            value={manualHost}
            onChange={(e) => setManualHost(e.target.value)}
          />
          <button
            type="button"
            className="btn primary"
            disabled={!manualHost.trim() || isUploading || isDownloading}
            onClick={() => {
              const h = manualHost.trim();
              beginSend(h, h);
            }}
          >
            Choose file…
          </button>
        </div>
      </section>

      <section className="devices">
        <div className="section-head">
          <h2>Nearby devices</h2>
          <span className="section-meta">{peers.length} on LAN</span>
        </div>

        {peers.length === 0 ? (
          <div className="empty">
            <p className="empty-title">Looking for peers…</p>
            <p className="empty-body">
              Open LocalBeam on another computer on the same Wi‑Fi. We use UDP broadcast and Bonjour-style discovery (mDNS)
              so mixed Windows and Mac setups can find each other more reliably.
            </p>
          </div>
        ) : (
          <ul className="device-list">
            {peers.map((peer) => (
              <li key={peerAddress(peer)} className="device-card">
                <div className="device-icon" aria-hidden="true" />
                <div className="device-meta">
                  <span className="device-name">{peer.hostname || 'Device'}</span>
                  <span className="device-addr">{peerAddress(peer)}</span>
                </div>
                <button
                  type="button"
                  className="btn primary sm"
                  disabled={isUploading || isDownloading}
                  onClick={() => beginSend(peerAddress(peer), peer.hostname || peer.ip)}
                >
                  Send
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>

      <footer className="foot">
        <span>LocalBeam · MIT</span>
        <span className="sep">·</span>
        <span>Tal</span>
        <span className="sep">·</span>
        <span>2026</span>
      </footer>
    </div>
  );
}

export default App;
