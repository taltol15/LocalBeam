import { useState, useEffect } from 'react';
import './App.css';
import { EventsOn } from "../wailsjs/runtime/runtime";
import { SelectFile, SendFile, GetMyPIN } from "../wailsjs/go/main/App"; 

function App() {
    const [peers, setPeers] = useState([]);
    const [myPin, setMyPin] = useState("....");
    const [notifications, setNotifications] = useState([]);
    
    // מצבי התקדמות
    const [uploadProgress, setUploadProgress] = useState(0);
    const [isUploading, setIsUploading] = useState(false);
    
    // --- הוספה: מצב קבלה ---
    const [downloadProgress, setDownloadProgress] = useState(0);
    const [isDownloading, setIsDownloading] = useState(false);

    useEffect(() => {
        GetMyPIN().then(pin => setMyPin(pin));

        EventsOn("peer-found", (peer) => {
            setPeers(list => {
                if (list.find(p => p.ip === peer.ip)) return list;
                return [...list, peer];
            });
        });

        EventsOn("file-received", (info) => {
            // סיימנו להוריד
            setIsDownloading(false);
            setDownloadProgress(0);

            addNotification({
                type: 'success',
                title: 'File Received! 📥',
                message: `${info.filename}`,
                sub: `From: ${info.sender}\nSize: ${info.size}`
            });
        });

        EventsOn("upload-progress", (percent) => {
            setUploadProgress(percent);
        });

        // --- הוספה: האזנה להתקדמות ההורדה ---
        EventsOn("download-progress", (percent) => {
            if (percent < 100) {
                setIsDownloading(true);
            }
            setDownloadProgress(percent);
        });

    }, []);

    const addNotification = (notif) => {
        const id = Date.now();
        setNotifications(prev => [...prev, { ...notif, id }]);
        setTimeout(() => {
            setNotifications(prev => prev.filter(n => n.id !== id));
        }, 5000);
    };

    const handleSendClick = async (ip) => {
        const filePath = await SelectFile();
        if (!filePath) return;

        const pin = prompt(`Enter security PIN for ${ip}:`);
        if (!pin) return;

        setIsUploading(true);
        setUploadProgress(0);
        
        const result = await SendFile(ip, filePath, pin);
        
        setIsUploading(false);
        setUploadProgress(0);

        if (result === "Success") {
            addNotification({ type: 'success', title: 'Sent Successfully! 🚀', message: 'File delivered.' });
        } else {
            addNotification({ type: 'error', title: 'Failed ❌', message: result });
        }
    };

    return (
        <div className="app-container">
            <div className="header">
                <h1>📡 LocalBeam</h1>
                <div className="my-pin-card">
                    <span>MY SECRET CODE</span>
                    <div className="pin-digits">{myPin}</div>
                </div>
            </div>

            {/* --- Progress Overlay: Sending --- */}
            {isUploading && (
                <div className="progress-overlay">
                    <div className="progress-box">
                        <h3>Sending File... 🚀</h3>
                        <div className="progress-bar-bg">
                            <div 
                                className="progress-bar-fill" 
                                style={{width: `${uploadProgress}%`}}
                            ></div>
                        </div>
                        <div className="progress-text">{uploadProgress}%</div>
                    </div>
                </div>
            )}

            {/* --- Progress Overlay: Receiving --- */}
            {isDownloading && (
                <div className="progress-overlay">
                    <div className="progress-box" style={{borderColor: '#4fd6be', boxShadow: '0 0 20px rgba(79, 214, 190, 0.3)'}}>
                        <h3 style={{color: '#4fd6be'}}>Incoming File... 📥</h3>
                        <div className="progress-bar-bg">
                            <div 
                                className="progress-bar-fill" 
                                style={{
                                    width: `${downloadProgress}%`,
                                    background: 'linear-gradient(90deg, #4fd6be, #2bb59a)' // צבע ירוק-טורקיז לקבלה
                                }}
                            ></div>
                        </div>
                        <div className="progress-text">{downloadProgress}%</div>
                    </div>
                </div>
            )}

            <div className="notifications-container">
                {notifications.map(n => (
                    <div key={n.id} className={`notification ${n.type}`}>
                        <strong>{n.title}</strong>
                        <div>{n.message}</div>
                        {n.sub && <div className="sub-text">{n.sub}</div>}
                    </div>
                ))}
            </div>

            <div className="radar-grid">
                {peers.map((peer, index) => (
                    <div key={index} className="peer-card">
                        <div className="peer-icon">🖥️</div>
                        <div className="peer-info">
                            <div className="peer-name">{peer.hostname}</div>
                            <div className="peer-ip">{peer.ip}</div>
                        </div>
                        <button onClick={() => handleSendClick(peer.ip)} disabled={isUploading || isDownloading}>
                            {isUploading ? 'Sending...' : 'Send File'}
                        </button>
                    </div>
                ))}
                
                {peers.length === 0 && <div className="scanning">Scanning Network...</div>}
                {/* --- הוספת ה-Footer כאן --- */}
            <div className="footer">
                <p>
                    Developed with ❤️ by <strong>Tal</strong> | Powered by <strong>LocalBeam.net</strong>
                    <br/>
                    <span style={{fontSize: "0.8em", opacity: 0.7}}>© 2025 | LocalBeam.</span>
                </p>
            </div>
            </div>
        </div>
    );
}

export default App;