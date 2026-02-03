import { useState, useEffect, useRef } from 'react';
import './App.css';
import { Initialize, SendMessage, JoinChannel, GetNodeID, GetRecentChats, GetHistory, KickUser, GetChannelMembers, StartDM, SendFile, DownloadAttachment, GetMessageTags } from "../wailsjs/go/main/App";
import { EventsOn } from "../wailsjs/runtime";
import SettingsModal from './SettingsModal';
import UserList from './UserList';
import ChannelView from './ChannelView';
import WelcomeDashboard from './WelcomeDashboard';
import { format } from 'date-fns';
import { Message } from './types';
import ChannelSettingsModal from './ChannelSettingsModal';

const getAvatarColor = (name: string) => {
    let hash = 0;
    for (let i = 0; i < name.length; i++) {
        hash = name.charCodeAt(i) + ((hash << 5) - hash);
    }
    const c = (hash & 0x00FFFFFF).toString(16).toUpperCase();
    return "#" + "00000".substring(0, 6 - c.length) + c;
};

function App() {
    const [messages, setMessages] = useState<Message[]>([]);
    const [input, setInput] = useState("");
    const [currentChannel, setCurrentChannel] = useState(""); // Default empty for Dashboard
    const [chats, setChats] = useState<any[]>([]);
    const [nodeID, setNodeID] = useState("");
    const [connStatus, setConnStatus] = useState<'idle' | 'connecting' | 'connected' | 'error'>('idle');
    const [errorMsg, setErrorMsg] = useState("");
    const [gridStatus, setGridStatus] = useState("Idle");
    const [settingsVisible, setSettingsVisible] = useState(false);
    const [channelSettingsVisible, setChannelSettingsVisible] = useState(false);
    const [showRoster, setShowRoster] = useState(true);
    const [members, setMembers] = useState<any[]>([]);
    const [theme, setTheme] = useState<'dark' | 'light'>('dark');

    const [isDragging, setIsDragging] = useState(false);
    const messagesEndRef = useRef<HTMLDivElement>(null);

    const scrollToBottom = () => {
        messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
    };

    const loadChats = async () => {
        const c = await GetRecentChats();
        setChats(c || []);
    };

    const loadMembers = async (channel: string) => {
        if (!channel || !channel.startsWith('#')) return;
        const mems = await GetChannelMembers(channel);
        setMembers(mems || []);
    };

    const loadData = async (target: string) => {
        try {
            const hist = await GetHistory(target);
            const tagsMap = await GetMessageTags(target);
            if (hist && hist.length > 0) {
                const mapped = hist.map((h: any) => ({
                    id: h.MessageID,
                    sender: h.SenderID,
                    content: h.Content,
                    channel: h.Target,
                    timestamp: new Date(h.Timestamp).getTime(),
                    mine: h.SenderID === nodeID,
                    tags: tagsMap ? tagsMap[h.MessageID] : []
                }));
                mapped.sort((a: any, b: any) => a.timestamp - b.timestamp);
                setMessages(mapped);
            }
            // Join only if connected
            if (connStatus === 'connected' && target.startsWith('#')) {
                await JoinChannel(target);
            }
        } catch (e) {
            console.error(e);
        }
    };

    const handleChannelClick = (ch: string) => {
        setCurrentChannel(ch);
        setMessages([]);
        loadData(ch);
        if (ch.startsWith('#')) {
            loadMembers(ch);
        } else {
            setMembers([]);
        }
    };

    const handleConnect = async () => {
        setConnStatus('connecting');
        setErrorMsg("");
        try {
            const profile = "UserGUI" + Math.floor(Math.random() * 1000);
            const res = await Initialize(profile);
            if (res === "") {
                // Success is actually handled by relay:connected event usually, 
                // but we can set it here if Initialize returns successfully.
                setConnStatus('connected');
                const id = await GetNodeID();
                setNodeID(id);
                loadChats();
                // Redireciona automaticamente para o canal geral após conectar
                handleChannelClick("#Inicio");
            } else {
                setConnStatus('error');
                setErrorMsg(res);
            }
        } catch (e: any) {
            setConnStatus('error');
            setErrorMsg(e.toString());
        }
    };

    const handleUserClick = async (targetNodeID: string) => {
        if (targetNodeID === nodeID) return;
        const dmID = await StartDM(targetNodeID);
        if (dmID) {
            await loadChats();
            handleChannelClick(dmID);
        }
    };

    useEffect(() => {
        scrollToBottom();
    }, [messages]);

    useEffect(() => {
        // Initial load of chats from local DB (Offline mode support)
        loadChats();

        // 2. Events
        EventsOn("relay:connected", () => {
            setConnStatus('connected');
            loadChats();
        });

        EventsOn("relay:disconnected", () => {
            setConnStatus('error');
            setErrorMsg("Disconectado da rede Relay.");
        });

        EventsOn("message:received", (payload: any) => {
            const newMsg: Message = {
                id: payload.ID,
                sender: payload.SenderID,
                content: payload.Content,
                channel: payload.TargetID,
                timestamp: new Date(payload.Timestamp).getTime(),
                mine: payload.SenderID === nodeID,
                tags: [] // Tags come enriched usually or via separate update
            };

            if (payload.TargetID === currentChannel) {
                setMessages(prev => [...prev, newMsg]);
                if (currentChannel.startsWith('#')) loadMembers(currentChannel);
            }
            loadChats();
        });

        EventsOn("grid:status", (status: string) => {
            setGridStatus(status);
        });

        EventsOn("wails:file-drop", async (files: string[]) => {
            if (files && files.length > 0 && currentChannel && connStatus === 'connected') {
                try {
                    await SendFile(currentChannel, files[0]);
                } catch (e) {
                    console.error("SendFile error:", e);
                }
            }
        });

        // UI Drag & Drop helpers...
        const handleDragOver = (e: any) => { e.preventDefault(); setIsDragging(true); };
        const handleDragLeave = (e: any) => { e.preventDefault(); setIsDragging(false); };
        window.addEventListener('dragover', handleDragOver);
        window.addEventListener('dragleave', handleDragLeave);

        return () => {
            window.removeEventListener('dragover', handleDragOver);
            window.removeEventListener('dragleave', handleDragLeave);
        };
    }, [nodeID, currentChannel, connStatus]);

    const handleChannelSendMessage = async (text: string) => {
        const tempID = "temp-" + Date.now();
        const newMsg: Message = {
            id: tempID,
            sender: "Me",
            content: text,
            channel: currentChannel,
            timestamp: Date.now(),
            mine: true,
            pending: true,
            tags: []
        };

        setMessages(prev => [...prev, newMsg]);

        try {
            await SendMessage(currentChannel, text);
            setMessages(prev => prev.map(m =>
                m.id === tempID ? { ...m, pending: false } : m
            ));
        } catch (e) {
            console.error("RPC error:", e);
        }
    };

    const handleNavigateHome = () => {
        setCurrentChannel("");
    };

    const handleDownload = async (fileID: string, fileName: string) => {
        try {
            const status = await DownloadAttachment(fileID, fileName);
            if (status === "success") alert(`✅ Arquivo salvo!`);
        } catch (e) { console.error(e); }
    };

    return (
        <div id="root" className={`app-container ${theme}`}>
            <div className="sidebar">
                <div className="sidebar-header" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', paddingRight: '15px' }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                        <div style={{ position: 'relative' }}>
                            <div style={{ width: '32px', height: '32px', background: '#ffd700', borderRadius: '50%', display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#000', fontWeight: 'bold' }}>
                                U
                            </div>
                            <div style={{
                                width: '10px',
                                height: '10px',
                                backgroundColor: connStatus === 'connected' ? '#44ff44' : '#ff4444',
                                borderRadius: '50%',
                                position: 'absolute',
                                bottom: 0,
                                right: 0,
                                border: '2px solid #1a1a1a'
                            }} />
                        </div>
                        <h2 style={{ fontSize: '1.2rem', margin: 0 }}>Enxame 🐝</h2>
                    </div>
                    <span onClick={() => setSettingsVisible(true)} style={{ cursor: 'pointer', fontSize: '1.2rem' }}>⚙️</span>
                </div>

                <div className="sidebar-nav">
                    <button
                        className={`nav-item ${currentChannel === "" ? "active" : ""}`}
                        onClick={handleNavigateHome}
                        style={{ width: '100%', textAlign: 'left', padding: '10px 15px', background: 'none', border: 'none', color: 'inherit', cursor: 'pointer', display: 'flex', alignItems: 'center', gap: '10px' }}
                    >
                        🏠 Dashboard
                    </button>
                </div>

                <div className="sidebar-section-title">Canais</div>

                <ul className="channel-list">
                    {chats.map((c: any) => {
                        const isInicio = c.ID === "#Inicio";
                        return (
                            <li key={c.ID}
                                className={`channel-item ${currentChannel === c.ID ? "active" : ""}`}
                                onClick={() => handleChannelClick(c.ID)}>
                                <div className="channel-avatar-container">
                                    {c.Avatar ? (
                                        <img src={c.Avatar} alt="" className="channel-avatar-img" />
                                    ) : (isInicio || c.Name === "Inicio") ? (
                                        <span className="channel-avatar-fallback">🏠</span>
                                    ) : (
                                        <div className="channel-avatar-letter" style={{ backgroundColor: getAvatarColor(c.Name) }}>
                                            {c.Name.substring(0, 1).toUpperCase()}
                                        </div>
                                    )}
                                </div>
                                <span className="channel-name">{c.Name}</span>
                                {c.UnreadCount > 0 && <span className="badge">{c.UnreadCount}</span>}
                            </li>
                        );
                    })}
                </ul>

                <div className="footer">
                    <div className="footer-status">
                        <span className={`status-indicator ${gridStatus === "Idle" ? "idle" : "syncing"}`}></span>
                        {gridStatus === "Idle" ? "Grid: Idle" : "Grid: Syncing..."}
                    </div>
                    <div style={{ fontSize: '0.8rem', opacity: 0.9, fontWeight: '500' }}>v1.0.1</div>
                </div>
            </div>

            <div className={`chat-area ${showRoster && currentChannel ? 'with-roster' : ''}`}>
                {currentChannel ? (
                    <ChannelView
                        channelID={currentChannel}
                        messages={messages.filter(m => m.channel === currentChannel)}
                        onSendMessage={handleChannelSendMessage}
                        onDownload={handleDownload}
                        onUserClick={handleUserClick}
                        onNavigateHome={handleNavigateHome}
                        onOpenSettings={() => setChannelSettingsVisible(true)}
                        isAdmin={true}
                        isOnline={connStatus === 'connected'}
                    />
                ) : (
                    <WelcomeDashboard
                        onConnectClick={handleConnect}
                        status={connStatus}
                        errorMsg={errorMsg}
                    />
                )}
            </div>

            {showRoster && currentChannel !== "" && currentChannel.startsWith('#') && (
                <UserList members={members} onUserClick={handleUserClick} />
            )}

            {isDragging && (
                <div className="drop-overlay">
                    <div className="drop-content">
                        <h2>📂 Solte o arquivo para enviar</h2>
                        <p>para {currentChannel}</p>
                    </div>
                </div>
            )}

            <SettingsModal
                visible={settingsVisible}
                onClose={() => setSettingsVisible(false)}
                nodeID={nodeID}
            />

            <ChannelSettingsModal
                visible={channelSettingsVisible}
                onClose={() => { setChannelSettingsVisible(false); loadChats(); }}
                channelID={currentChannel}
            />
        </div>
    );
}

export default App;
