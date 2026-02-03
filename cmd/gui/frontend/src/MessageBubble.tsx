import { format } from 'date-fns';
import { Message, ChannelTag } from './types';
import { useState } from 'react';

interface MessageBubbleProps {
    msg: Message;
    onDownload: (fileID: string, fileName: string) => void;
    onAddTag: (msgID: string, tagID: string) => void;
    availableTags: ChannelTag[];
}

export default function MessageBubble({ msg, onDownload, onAddTag, availableTags }: MessageBubbleProps) {
    const [contextMenu, setContextMenu] = useState<{ x: number, y: number, visible: boolean }>({ x: 0, y: 0, visible: false });

    const handleContextMenu = (e: React.MouseEvent) => {
        e.preventDefault();
        setContextMenu({ x: e.pageX, y: e.pageY, visible: true });
    };

    const handleTagClick = (tagID: string) => {
        onAddTag(msg.id, tagID);
        setContextMenu({ ...contextMenu, visible: false });
    };

    // Close menu on click elsewhere (handled by global listener in App usually, but adding local backdrop for simplicity if needed, or rely on blur)
    // For now, simpler: self-closing on selection only. 
    // Ideally App.tsx handles global click to close. We'll use a local backdrop overlay for this component's menu context.

    return (
        <div
            className={`message ${msg.mine ? "sent" : "received"}`}
            onContextMenu={handleContextMenu}
            style={{ position: 'relative' }}
        >
            {/* Context Menu */}
            {contextMenu.visible && (
                <>
                    <div style={{ position: 'fixed', top: 0, left: 0, right: 0, bottom: 0, zIndex: 999 }} onClick={() => setContextMenu({ ...contextMenu, visible: false })} />
                    <div className="context-menu" style={{
                        position: 'fixed',
                        top: contextMenu.y,
                        left: contextMenu.x,
                        background: 'var(--bg-secondary)',
                        border: '1px solid var(--border-color)',
                        borderRadius: '8px',
                        boxShadow: '0 4px 12px rgba(0,0,0,0.5)',
                        zIndex: 1000,
                        padding: '5px',
                        minWidth: '150px'
                    }}>
                        <div style={{ padding: '8px', borderBottom: '1px solid var(--border-color)', fontWeight: 'bold' }}>Adicionar Tag 🏷️</div>
                        {availableTags.length === 0 ? (
                            <div style={{ padding: '8px', color: 'gray' }}>Nenhuma tag criada</div>
                        ) : (
                            availableTags.map(tag => (
                                <div
                                    key={tag.ID}
                                    onClick={() => handleTagClick(tag.ID)}
                                    style={{
                                        padding: '8px',
                                        cursor: 'pointer',
                                        display: 'flex',
                                        alignItems: 'center',
                                        gap: '8px',
                                        transition: 'background 0.2s'
                                    }}
                                    onMouseEnter={(e) => e.currentTarget.style.background = 'var(--bg-tertiary)'}
                                    onMouseLeave={(e) => e.currentTarget.style.background = 'transparent'}
                                >
                                    <span style={{ width: '10px', height: '10px', borderRadius: '50%', background: tag.Color }}></span>
                                    {tag.Name}
                                </div>
                            ))
                        )}
                    </div>
                </>
            )}

            <div className="message-content">
                {!msg.mine && <div className="sender-name">{msg.sender.substring(0, 8)}</div>}

                {/* Content Rendering */}
                {msg.content.startsWith('[FILE:') ? (
                    (() => {
                        const parts = msg.content.slice(6, -1).split(':');
                        const fileName = parts[1];
                        const size = parseInt(parts[2]);
                        return (
                            <div className="file-attachment">
                                <div className="file-icon">📎</div>
                                <div className="file-info">
                                    <span className="file-name">{fileName}</span>
                                    <span className="file-size">{(size / 1024).toFixed(1)} KB</span>
                                </div>
                                <button
                                    className="download-btn"
                                    onClick={(e) => {
                                        e.stopPropagation();
                                        onDownload(parts[0], fileName);
                                    }}
                                >
                                    ⬇️
                                </button>
                            </div>
                        );
                    })()
                ) : (
                    msg.content
                )}

                {/* Tags rendering (Pills) */}
                {msg.tags && msg.tags.length > 0 && (
                    <div style={{ display: 'flex', flexWrap: 'wrap', gap: '4px', marginTop: '6px' }}>
                        {msg.tags.map((tag, idx) => (
                            <span
                                key={`${tag.TagID}-${idx}`}
                                style={{
                                    background: tag.TagColor + '40', // 25% opacity
                                    color: 'var(--text-primary)',
                                    border: `1px solid ${tag.TagColor}`,
                                    borderRadius: '12px',
                                    padding: '2px 8px',
                                    fontSize: '0.75rem',
                                    display: 'flex',
                                    alignItems: 'center',
                                    gap: '4px'
                                }}
                            >
                                <span style={{ width: '6px', height: '6px', borderRadius: '50%', background: tag.TagColor }}></span>
                                {tag.TagName}
                            </span>
                        ))}
                    </div>
                )}
            </div>

            <div className="message-footer">
                <span className="timestamp">{format(msg.timestamp, 'HH:mm')}</span>
                {msg.mine && (
                    <span style={{ marginLeft: '4px', fontSize: '10px' }}>
                        {msg.pending ? "🕒" : "✓"}
                    </span>
                )}
            </div>
        </div>
    );
}
