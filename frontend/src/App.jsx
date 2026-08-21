import {useEffect, useState, useCallback} from 'react';
import './App.css';
import {EventsOn} from '../wailsjs/runtime/runtime';
import {
  SelectFile,
  SendFile,
  GetMyPIN,
  GetFingerprint,
  ProtocolInfo,
  RespondToOffer,
  GetAuditLog,
} from '../wailsjs/go/main/App';

function peerAddress(peer) {
  const port = peer.port ?? 34567;
  return port === 34567 ? peer.ip : `${peer.ip}:${port}`;
}

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

function App() {
  const [peers, setPeers] = useState([]);
  const [myPin, setMyPin] = useState('······');
  const [fingerprint, setFingerprint] = useState('');
  const [protocolVer, setProtocolVer] = useState('');
  const [notifications, setNotifications] = useState([]);

  const [uploadProgress, setUploadProgress] = useState(0);
  const [isUploading, setIsUploading] = useState(false);
  const [waitingAccept, setWaitingAccept] = useState(false);

  const [downloadProgress, setDownloadProgress] = useState(0);
  const [isDownloading, setIsDownloading] = useState(false);

  const [manualHost, setManualHost] = useState('');
  const [senderName, setSenderName] = useState(() => localStorage.getItem('lb_name') || '');
  const [senderEmail, setSenderEmail] = useState(() => localStorage.getItem('lb_email') || '');

  const [sendModal, setSendModal] = useState(null);
  const [pinInput, setPinInput] = useState('');

  const [incomingOffer, setIncomingOffer] = useState(null);
  const [auditLog, setAuditLog] = useState([]);

  const addNotification = useCallback((notif) => {
    const id = Date.now() + Math.random();
    setNotifications((prev) => [...prev, {...notif, id}]);
    setTimeout(() => {
      setNotifications((prev) => prev.filter((n) => n.id !== id));
    }, 5200);
  }, []);

  const refreshAudit = useCallback(() => {
    GetAuditLog()
      .then((raw) => {
        try {
          setAuditLog(JSON.parse(raw || '[]'));
        } catch {
          setAuditLog([]);
        }
      })
      .catch(() => setAuditLog([]));
  }, []);

  useEffect(() => {
    GetMyPIN().then(setMyPin);
    GetFingerprint().then(setFingerprint).catch(() => {});
    ProtocolInfo().then(setProtocolVer).catch(() => {});
    refreshAudit();

    EventsOn('peer-found', (peer) => {
      setPeers((list) => {
        const addr = peerAddress(peer);
        if (list.some((p) => peerAddress(p) === addr)) {
          return list.map((p) => (peerAddress(p) === addr ? {...p, ...peer} : p));
        }
        return [...list, peer];
      });
    });

    EventsOn('pin-updated', (pin) => {
      if (pin) setMyPin(pin);
    });

    EventsOn('incoming-offer', (offer) => {
      setIncomingOffer(offer);
    });

    EventsOn('waiting-accept', () => {
      setWaitingAccept(true);
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
      refreshAudit();
    });

    EventsOn('upload-progress', (percent) => {
      setWaitingAccept(false);
      setUploadProgress(percent);
    });

    EventsOn('download-progress', (percent) => {
      if (percent < 100) setIsDownloading(true);
      setDownloadProgress(percent);
      if (percent >= 100) setIsDownloading(false);
    });
  }, [addNotification, refreshAudit]);

  const copyPin = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(myPin);
      addNotification({type: 'info', title: 'Copied', message: 'PIN copied to clipboard.'});
    } catch {
      addNotification({type: 'error', title: 'Copy failed', message: 'Could not access clipboard.'});
    }
  }, [myPin, addNotification]);

  const beginSend = useCallback(
    async (address, displayName) => {
      if (!senderName.trim() || !EMAIL_RE.test(senderEmail.trim())) {
        addNotification({
          type: 'error',
          title: 'Identity required',
          message: 'Enter your name and organizational email before sending.',
        });
        return;
      }
      const filePath = await SelectFile();
      if (!filePath) return;
      setPinInput('');
      setSendModal({address, displayName, filePath});
    },
    [senderName, senderEmail, addNotification],
  );

  const confirmSend = useCallback(async () => {
    if (!sendModal) return;
    const pin = pinInput.trim();
    if (!/^\d{6}$/.test(pin)) {
      addNotification({
        type: 'error',
        title: 'Invalid PIN',
        message: 'Enter the 6-digit code shown on the receiver.',
      });
      return;
    }

    localStorage.setItem('lb_name', senderName.trim());
    localStorage.setItem('lb_email', senderEmail.trim().toLowerCase());

    const {address, displayName, filePath} = sendModal;
    setSendModal(null);
    setIsUploading(true);
    setWaitingAccept(true);
    setUploadProgress(0);

    const result = await SendFile(
      address,
      filePath,
      pin,
      senderName.trim(),
      senderEmail.trim().toLowerCase(),
    );

    setIsUploading(false);
    setWaitingAccept(false);
    setUploadProgress(0);
    refreshAudit();

    if (result === 'Success') {
      addNotification({type: 'success', title: 'Sent', message: `Delivered to ${displayName}.`});
    } else {
      addNotification({
        type: 'error',
        title: 'Send failed',
        message: result.replace(/^Error:\s*/, ''),
      });
    }
  }, [sendModal, pinInput, senderName, senderEmail, addNotification, refreshAudit]);

  const handleOffer = useCallback(
    async (accept) => {
      if (!incomingOffer) return;
      const id = incomingOffer.id;
      setIncomingOffer(null);
      const res = await RespondToOffer(id, accept);
      if (accept && res === 'Accepted') {
        addNotification({
          type: 'info',
          title: 'Accepted',
          message: 'Waiting for encrypted upload…',
        });
      } else if (!accept) {
        addNotification({type: 'info', title: 'Rejected', message: 'Transfer declined.'});
      } else {
        addNotification({type: 'error', title: 'Offer error', message: res});
      }
      GetMyPIN().then(setMyPin);
      refreshAudit();
    },
    [incomingOffer, addNotification, refreshAudit],
  );

  return (
    <div className="app">
      <div className="bg-grid" aria-hidden="true" />

      <header className="top">
        <div className="brand">
          <img className="brand-mark" src="/labs-mark.svg" alt="" width={44} height={44} />
          <div>
            <h1>LocalBeam</h1>
            <p className="tagline">LAN file transfer · Trustity Labs</p>
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
          <p className="pin-hint">
            6-digit PIN · TLS
            {fingerprint ? (
              <>
                {' '}
                · fp <code className="fp">{fingerprint}</code>
              </>
            ) : null}
          </p>
        </div>
      </header>

      <section className="identity">
        <h2>Your identity (for audit)</h2>
        <p className="section-desc">
          Claimed name and organizational email are logged locally on the receiver. Not cryptographically proven.
        </p>
        <div className="identity-row">
          <input
            className="text-input"
            placeholder="Full name"
            value={senderName}
            onChange={(e) => setSenderName(e.target.value)}
            autoComplete="name"
          />
          <input
            className="text-input"
            placeholder="you@company.com"
            value={senderEmail}
            onChange={(e) => setSenderEmail(e.target.value)}
            autoComplete="email"
            type="email"
          />
        </div>
      </section>

      {isUploading && (
        <div className="modal-backdrop" role="dialog" aria-label="Upload progress">
          <div className="modal sheet">
            <h2>{waitingAccept ? 'Waiting for accept…' : 'Sending (encrypted)'}</h2>
            <div className="progress-track">
              <div className="progress-fill send" style={{width: `${uploadProgress}%`}} />
            </div>
            <p className="progress-label">
              {waitingAccept ? 'Receiver must approve the transfer' : `${uploadProgress}%`}
            </p>
          </div>
        </div>
      )}

      {isDownloading && (
        <div className="modal-backdrop" role="dialog" aria-label="Download progress">
          <div className="modal sheet">
            <h2>Receiving (decrypting)</h2>
            <div className="progress-track">
              <div className="progress-fill recv" style={{width: `${downloadProgress}%`}} />
            </div>
            <p className="progress-label">{downloadProgress}%</p>
          </div>
        </div>
      )}

      {sendModal && (
        <div className="modal-backdrop" role="dialog" aria-modal="true" aria-labelledby="pin-title">
          <div className="modal sheet pin-sheet">
            <h2 id="pin-title">Receiver PIN</h2>
            <p className="modal-sub">Enter the 6-digit PIN shown on {sendModal.displayName}.</p>
            <input
              className="pin-field"
              type="text"
              inputMode="numeric"
              maxLength={6}
              placeholder="000000"
              autoFocus
              value={pinInput}
              onChange={(e) => setPinInput(e.target.value.replace(/\D/g, '').slice(0, 6))}
              onKeyDown={(e) => e.key === 'Enter' && confirmSend()}
            />
            <div className="modal-actions">
              <button type="button" className="btn ghost" onClick={() => setSendModal(null)}>
                Cancel
              </button>
              <button type="button" className="btn primary" onClick={confirmSend}>
                Offer transfer
              </button>
            </div>
          </div>
        </div>
      )}

      {incomingOffer && (
        <div className="modal-backdrop" role="dialog" aria-modal="true" aria-labelledby="offer-title">
          <div className="modal sheet offer-sheet">
            <h2 id="offer-title">Incoming transfer</h2>
            <p className="modal-sub tip">{incomingOffer.tip}</p>
            <dl className="offer-meta">
              <div>
                <dt>File</dt>
                <dd>
                  {incomingOffer.filename}{' '}
                  <span className="muted">({incomingOffer.size_label})</span>
                </dd>
              </div>
              <div>
                <dt>Claimed sender</dt>
                <dd>
                  {incomingOffer.sender_name}
                  <br />
                  <span className="muted">{incomingOffer.sender_email}</span>
                </dd>
              </div>
              <div>
                <dt>Peer</dt>
                <dd>{incomingOffer.peer_ip}</dd>
              </div>
            </dl>
            <div className="modal-actions">
              <button type="button" className="btn ghost danger" onClick={() => handleOffer(false)}>
                Reject
              </button>
              <button type="button" className="btn primary" onClick={() => handleOffer(true)}>
                Accept
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

      <div className="main-scroll">
        <section className="manual">
          <h2>Send by address</h2>
          <p className="section-desc">
            If a device does not appear, enter its IP (optional <code>:port</code>).
          </p>
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
                Open LocalBeam on another computer on the same Wi‑Fi. Discovery uses UDP broadcast and
                mDNS. Transfers use HTTPS + AES-GCM with PIN challenge and manual accept.
              </p>
            </div>
          ) : (
            <ul className="device-list">
              {peers.map((peer) => (
                <li key={peerAddress(peer)} className="device-card">
                  <div className="device-icon" aria-hidden="true" />
                  <div className="device-meta">
                    <span className="device-name">{peer.hostname || 'Device'}</span>
                    <span className="device-addr">
                      {peerAddress(peer)}
                      {peer.fingerprint ? ` · fp ${peer.fingerprint}` : ''}
                      {peer.version ? ` · v${peer.version}` : ''}
                    </span>
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

        <section className="audit">
          <div className="section-head">
            <h2>Transfer log</h2>
            <button type="button" className="btn ghost sm" onClick={refreshAudit}>
              Refresh
            </button>
          </div>
          <p className="section-desc">Local audit only — stored on this machine.</p>
          {auditLog.length === 0 ? (
            <p className="empty-body">No transfers yet.</p>
          ) : (
            <ul className="audit-list">
              {auditLog.map((e, i) => (
                <li key={`${e.timestamp}-${i}`}>
                  <span className={`audit-result ${e.result}`}>{e.result}</span>
                  <span className="audit-main">
                    {e.filename} · {e.name} &lt;{e.email}&gt;
                  </span>
                  <span className="audit-meta">
                    {e.direction} · {e.peer_ip} · {e.timestamp}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </section>
      </div>

      <footer className="foot">
        <span>LocalBeam is a Trustity Labs experiment · not a production Trustity product</span>
        <span className="sep">·</span>
        <span>MIT</span>
      </footer>
    </div>
  );
}

export default App;
