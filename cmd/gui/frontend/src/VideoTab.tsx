import { useState, useEffect } from 'react';
import { SetChannelConfig, GetChannelConfig } from "../wailsjs/go/main/App";

interface VideoTabProps {
    channelID: string;
    isAdmin: boolean;
}

export default function VideoTab({ channelID, isAdmin }: VideoTabProps) {
    const [videoUrl, setVideoUrl] = useState("");
    const [inputUrl, setInputUrl] = useState("");

    const loadConfig = async () => {
        try {
            const url = await GetChannelConfig(channelID, "video_source_url");
            if (url) {
                setVideoUrl(url);
                setInputUrl(url);
            }
        } catch (e) {
            console.error(e);
        }
    };

    useEffect(() => {
        setVideoUrl("");
        setInputUrl("");
        loadConfig();
    }, [channelID]);

    const handleSave = async () => {
        if (!inputUrl) return;
        await SetChannelConfig(channelID, "video_source_url", inputUrl);
        setVideoUrl(inputUrl);
        alert("Configuração salva (Admin Only).");
    };

    const getEmbedUrl = (url: string) => {
        if (!url) return "";
        // Simple YouTube converter
        // https://www.youtube.com/watch?v=VIDEO_ID -> https://www.youtube.com/embed/VIDEO_ID
        // https://youtu.be/VIDEO_ID -> https://www.youtube.com/embed/VIDEO_ID

        try {
            if (url.includes("youtube.com/watch")) {
                const urlParams = new URLSearchParams(new URL(url).search);
                const vid = urlParams.get("v");
                if (vid) return `https://www.youtube.com/embed/${vid}`;
            }
            if (url.includes("youtu.be/")) {
                const parts = url.split("/");
                const vid = parts[parts.length - 1];
                if (vid) return `https://www.youtube.com/embed/${vid}`;
            }
        } catch (e) {
            console.error("URL Parsing error", e);
        }
        return url; // Return as is if not matched (maybe it's already an embed link)
    };

    const embedSrc = getEmbedUrl(videoUrl);

    return (
        <div className="video-tab" style={{ display: 'flex', flexDirection: 'column', height: '100%', padding: '20px', color: 'var(--text-primary)' }}>

            {/* Player Area */}
            <div className="video-player" style={{ flex: 1, background: '#000', borderRadius: '8px', overflow: 'hidden', position: 'relative' }}>
                {embedSrc ? (
                    <iframe
                        src={embedSrc}
                        style={{ width: '100%', height: '100%', border: 'none' }}
                        allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
                        allowFullScreen
                    />
                ) : (
                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%', color: '#666' }}>
                        <p>Nenhum vídeo configurado.</p>
                    </div>
                )}
            </div>

            {/* Admin Controls */}
            {isAdmin && (
                <div className="admin-controls" style={{ marginTop: '20px', padding: '15px', background: 'var(--bg-secondary)', borderRadius: '8px' }}>
                    <h4>⚙️ Configuração do Canal (Admin)</h4>
                    <div style={{ display: 'flex', gap: '10px', marginTop: '10px' }}>
                        <input
                            type="text"
                            value={inputUrl}
                            onChange={e => setInputUrl(e.target.value)}
                            placeholder="Cole a URL do YouTube aqui..."
                            style={{ flex: 1, padding: '8px' }}
                        />
                        <button onClick={handleSave}>Salvar</button>
                    </div>
                </div>
            )}
        </div>
    );
}
