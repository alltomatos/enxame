import { useState, useEffect } from 'react';
import { CreateTag, GetTags } from "../wailsjs/go/main/App";
import { ChannelTag, Message, MessageTag } from './types';
import { format } from 'date-fns';

interface TopicsTabProps {
    channelID: string;
    messages: Message[];
    isAdmin: boolean;
}

export default function TopicsTab({ channelID, messages, isAdmin }: TopicsTabProps) {
    const [tags, setTags] = useState<ChannelTag[]>([]);
    const [selectedTagID, setSelectedTagID] = useState<string | null>(null);
    const [newTagName, setNewTagName] = useState("");
    const [newTagColor, setNewTagColor] = useState("#3498db");

    // Load Tags
    const loadTags = async () => {
        const t = await GetTags(channelID);
        if (t) setTags(t);
    };

    useEffect(() => {
        loadTags();
        // Poll for tag updates or rely on parent reload? 
        // For MVP, polling or manual reload. We added events to SDK but need to bridge them efficiently.
        // Let's rely on tab switch or manual refresh for now, or simple poll.
        const interval = setInterval(loadTags, 5000);
        return () => clearInterval(interval);
    }, [channelID]);

    const handleCreateTag = async () => {
        if (!newTagName.trim()) return;
        await CreateTag(channelID, newTagName, newTagColor);
        setNewTagName("");
        loadTags();
    };

    // Filter Messages
    const filteredMessages = selectedTagID
        ? messages.filter(m => m.tags?.some(t => t.TagID === selectedTagID))
        : [];

    return (
        <div style={{ display: 'flex', height: '100%', flexDirection: 'column' }}>
            {/* Sidebar / Header area for Tags */}
            <div style={{ padding: '20px', borderBottom: '1px solid var(--border-color)', background: 'var(--bg-secondary)' }}>
                {isAdmin && (
                    <div style={{ display: 'flex', gap: '10px', marginBottom: '20px' }}>
                        <input
                            placeholder="Nome da Tag"
                            value={newTagName}
                            onChange={e => setNewTagName(e.target.value)}
                            style={{ padding: '8px', borderRadius: '4px', border: '1px solid var(--border-color)' }}
                        />
                        <input
                            type="color"
                            value={newTagColor}
                            onChange={e => setNewTagColor(e.target.value)}
                            style={{ height: '35px', width: '50px', padding: 0, border: 'none' }}
                        />
                        <button onClick={handleCreateTag}>Criar Tag</button>
                    </div>
                )}

                <div style={{ display: 'flex', flexWrap: 'wrap', gap: '10px' }}>
                    <button
                        onClick={() => setSelectedTagID(null)}
                        style={{
                            background: !selectedTagID ? 'var(--accent-primary)' : 'transparent',
                            border: '1px solid var(--border-color)',
                            color: !selectedTagID ? '#fff' : 'var(--text-primary)',
                            borderRadius: '16px', padding: '5px 15px'
                        }}
                    >
                        Todas
                    </button>
                    {tags.map(tag => (
                        <button
                            key={tag.ID}
                            onClick={() => setSelectedTagID(tag.ID === selectedTagID ? null : tag.ID)}
                            style={{
                                background: selectedTagID === tag.ID ? tag.Color : 'transparent',
                                border: `1px solid ${tag.Color}`,
                                color: selectedTagID === tag.ID ? '#fff' : 'var(--text-primary)',
                                borderRadius: '16px', padding: '5px 15px',
                                display: 'flex', alignItems: 'center', gap: '5px'
                            }}
                        >
                            <span style={{ width: '8px', height: '8px', borderRadius: '50%', background: selectedTagID === tag.ID ? '#fff' : tag.Color }}></span>
                            {tag.Name}
                        </button>
                    ))}
                </div>
            </div>

            {/* Filtered Messages View */}
            <div style={{ flex: 1, padding: '20px', overflowY: 'auto' }}>
                {!selectedTagID ? (
                    <div style={{ textAlign: 'center', color: 'gray', marginTop: '50px' }}>
                        Selecione uma tag para filtrar mensagens.
                        <br />
                        <small>Total de tags: {tags.length}</small>
                    </div>
                ) : (
                    <div>
                        <h4>Mensagens filtradas: {filteredMessages.length}</h4>
                        {filteredMessages.length === 0 ? (
                            <p>Nenhuma mensagem com esta tag.</p>
                        ) : (
                            filteredMessages.map(msg => (
                                <div key={msg.id} style={{
                                    background: 'var(--bg-secondary)',
                                    padding: '10px',
                                    borderRadius: '8px',
                                    marginBottom: '10px',
                                    borderLeft: `4px solid ${tags.find(t => t.ID === selectedTagID)?.Color}`
                                }}>
                                    <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '5px', fontSize: '0.9rem', opacity: 0.8 }}>
                                        <strong>{msg.sender.substring(0, 8)}</strong>
                                        <span>{format(msg.timestamp, 'dd/MM HH:mm')}</span>
                                    </div>
                                    <div>{msg.content}</div>
                                </div>
                            ))
                        )}
                    </div>
                )}
            </div>
        </div>
    );
}
