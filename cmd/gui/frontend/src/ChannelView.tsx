import { useState, useRef, useEffect } from 'react';
import WikiTab from './WikiTab';
import VideoTab from './VideoTab';
import TopicsTab from './TopicsTab';
import MessageBubble from './MessageBubble';
import { TagMessage, GetTags } from "../wailsjs/go/main/App";
import { Message, ChannelTag } from './types';

interface ChannelViewProps {
    channelID: string;
    messages: Message[];
    onSendMessage: (text: string) => void;
    onDownload: (fileID: string, fileName: string) => void;
    onUserClick: (userID: string) => void;
    onNavigateHome: () => void;
    onOpenSettings?: () => void;
    isAdmin: boolean;
    isOnline: boolean;
}

export default function ChannelView({ channelID, messages, onSendMessage, onDownload, onNavigateHome, onOpenSettings, isAdmin, isOnline }: ChannelViewProps) {
    const [activeTab, setActiveTab] = useState<'chat' | 'wiki' | 'video' | 'topics'>('chat');
    const [input, setInput] = useState("");
    const [availableTags, setAvailableTags] = useState<ChannelTag[]>([]);
    const messagesEndRef = useRef<HTMLDivElement>(null);

    const scrollToBottom = () => {
        messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
    };

    useEffect(() => {
        if (activeTab === 'chat') {
            scrollToBottom();
        }
    }, [messages, activeTab]);

    // Load Tags for Context Menu
    const loadTags = async () => {
        const t = await GetTags(channelID);
        if (t) setAvailableTags(t);
    };

    useEffect(() => {
        loadTags();
        // Poll for tags (simple sync)
        const interval = setInterval(loadTags, 5000);
        return () => clearInterval(interval);
    }, [channelID]);

    const handleKeyDown = (e: React.KeyboardEvent) => {
        if (e.key === 'Enter') handleSend();
    };

    const handleSend = () => {
        if (!input.trim() || !isOnline) return;
        onSendMessage(input);
        setInput("");
    };

    const handleAddTag = async (msgID: string, tagID: string) => {
        if (!isOnline) {
            alert("Você precisa estar online para adicionar tags.");
            return;
        }
        await TagMessage(channelID, msgID, tagID);
    };

    return (
        <div className="channel-view" style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
            {/* Header / Tabs */}
            <div className="channel-header" style={{
                borderBottom: '1px solid var(--border-color)',
                padding: '10px 20px',
                background: 'var(--bg-secondary)',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between'
            }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                    <button
                        onClick={onNavigateHome}
                        style={{ background: 'transparent', border: 'none', fontSize: '1.2rem', cursor: 'pointer', padding: '5px' }}
                        title="Voltar ao Dashboard"
                    >
                        🏠
                    </button>
                    <h3 style={{ margin: 0 }}>{channelID}</h3>
                    {isAdmin && onOpenSettings && (
                        <span
                            onClick={onOpenSettings}
                            style={{ cursor: 'pointer', fontSize: '1rem', opacity: 0.7 }}
                            title="Configurações do Canal"
                        >
                            ⚙️
                        </span>
                    )}
                </div>

                <div className="tabs" style={{ display: 'flex', gap: '15px' }}>
                    <button
                        className={`tab-btn ${activeTab === 'chat' ? 'active' : ''}`}
                        onClick={() => setActiveTab('chat')}
                        style={{ background: 'none', border: 'none', color: activeTab === 'chat' ? 'var(--accent-primary)' : 'var(--text-secondary)', cursor: 'pointer', fontWeight: 'bold' }}
                    >
                        💬 Chat
                    </button>
                    <button
                        className={`tab-btn ${activeTab === 'topics' ? 'active' : ''}`}
                        onClick={() => setActiveTab('topics')}
                        style={{ background: 'none', border: 'none', color: activeTab === 'topics' ? 'var(--accent-primary)' : 'var(--text-secondary)', cursor: 'pointer', fontWeight: 'bold' }}
                    >
                        🏷️ Tópicos
                    </button>
                    <button
                        className={`tab-btn ${activeTab === 'wiki' ? 'active' : ''}`}
                        onClick={() => setActiveTab('wiki')}
                        style={{ background: 'none', border: 'none', color: activeTab === 'wiki' ? 'var(--accent-primary)' : 'var(--text-secondary)', cursor: 'pointer', fontWeight: 'bold' }}
                    >
                        📚 Wiki
                    </button>
                    <button
                        className={`tab-btn ${activeTab === 'video' ? 'active' : ''}`}
                        onClick={() => setActiveTab('video')}
                        style={{ background: 'none', border: 'none', color: activeTab === 'video' ? 'var(--accent-primary)' : 'var(--text-secondary)', cursor: 'pointer', fontWeight: 'bold' }}
                    >
                        📺 Vídeos
                    </button>
                </div>
            </div>

            {/* Content Area */}
            <div className="channel-content" style={{ flex: 1, overflow: 'hidden', position: 'relative' }}>

                {/* 1. CHAT TAB */}
                <div className="tab-content-chat" style={{
                    display: activeTab === 'chat' ? 'flex' : 'none',
                    flexDirection: 'column',
                    height: '100%'
                }}>
                    <div className="messages-list" style={{ flex: 1, overflowY: 'auto', padding: '20px' }}>
                        {messages.map((msg) => (
                            <MessageBubble
                                key={msg.id}
                                msg={msg}
                                onDownload={onDownload}
                                onAddTag={handleAddTag}
                                availableTags={availableTags}
                            />
                        ))}
                        <div ref={messagesEndRef} />
                    </div>

                    <div className="input-area" style={{ padding: '20px', borderTop: '1px solid var(--border-color)' }}>
                        <input
                            className="chat-input"
                            value={input}
                            onChange={(e) => setInput(e.target.value)}
                            onKeyDown={handleKeyDown}
                            placeholder={isOnline ? `Message ${channelID}...` : `🔴 Conecte-se para enviar mensagens`}
                            disabled={!isOnline}
                            autoFocus
                        />
                        <button className="send-btn" onClick={handleSend} disabled={!isOnline}>Send</button>
                    </div>
                </div>

                {/* 2. TOPICS TAB */}
                <div className="tab-content-topics" style={{ display: activeTab === 'topics' ? 'block' : 'none', height: '100%', overflowY: 'auto' }}>
                    <TopicsTab channelID={channelID} messages={messages} isAdmin={isAdmin} />
                </div>

                {/* 3. WIKI TAB */}
                <div className="tab-content-wiki" style={{ display: activeTab === 'wiki' ? 'block' : 'none', height: '100%', overflowY: 'auto' }}>
                    <WikiTab channelID={channelID} />
                </div>

                {/* 4. VIDEO TAB */}
                <div className="tab-content-video" style={{ display: activeTab === 'video' ? 'block' : 'none', height: '100%', overflowY: 'auto' }}>
                    <VideoTab channelID={channelID} isAdmin={isAdmin} />
                </div>

            </div>
        </div>
    );
}
