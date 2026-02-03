import MDEditor from '@uiw/react-md-editor';
import { useState, useEffect } from 'react';
import { GetWikiPages, SaveWikiPage } from "../wailsjs/go/main/App";

interface WikiTabProps {
    channelID: string;
}

interface WikiPage {
    ID: string;
    Title: string;
    Content: string;
    UpdatedAt: string;
}

export default function WikiTab({ channelID }: WikiTabProps) {
    const [pages, setPages] = useState<WikiPage[]>([]);
    const [selectedPage, setSelectedPage] = useState<WikiPage | null>(null);
    const [isEditing, setIsEditing] = useState(false);
    const [searchTerm, setSearchTerm] = useState("");

    // Editor State
    const [editTitle, setEditTitle] = useState("");
    const [editContent, setEditContent] = useState<string | undefined>("");

    // Determine theme (simple check for now, can be improved to use context)
    const colorMode = document.documentElement.getAttribute('data-color-mode') || 'dark';

    const loadPages = async () => {
        try {
            const list = await GetWikiPages(channelID);
            setPages(list || []);
            // If selected page exists, update it
            if (selectedPage) {
                const updated = (list || []).find((p: any) => p.ID === selectedPage.ID);
                if (updated) setSelectedPage(updated);
            }
        } catch (e) {
            console.error(e);
        }
    };

    useEffect(() => {
        loadPages();
    }, [channelID]);

    const handleCreate = () => {
        setSelectedPage(null);
        setEditTitle("Nova Página");
        setEditContent("# Título\n\nEscreva aqui...");
        setIsEditing(true);
    };

    const handleSave = async () => {
        if (!editTitle.trim()) return;

        await SaveWikiPage(channelID, editTitle, editContent || "");
        setIsEditing(false);
        loadPages();
    };

    const handleSelect = (p: WikiPage) => {
        setSelectedPage(p);
        setIsEditing(false);
    };

    const handleEdit = () => {
        if (!selectedPage) return;
        setEditTitle(selectedPage.Title);
        setEditContent(selectedPage.Content);
        setIsEditing(true);
    };

    const filteredPages = pages.filter(p =>
        p.Title.toLowerCase().includes(searchTerm.toLowerCase())
    );

    return (
        <div className="wiki-tab" data-color-mode={colorMode} style={{ display: 'flex', height: '100%', color: 'var(--text-primary)' }}>
            {/* Sidebar List */}
            <div className="wiki-sidebar" style={{ width: '250px', borderRight: '1px solid var(--border-color)', padding: '10px', display: 'flex', flexDirection: 'column' }}>
                <div style={{ marginBottom: '10px' }}>
                    <input
                        type="text"
                        placeholder="Buscar páginas..."
                        value={searchTerm}
                        onChange={(e) => setSearchTerm(e.target.value)}
                        style={{ width: '100%', padding: '8px', marginBottom: '8px', background: 'var(--bg-secondary)', border: '1px solid var(--border-color)', color: 'var(--text-primary)' }}
                    />
                    <button onClick={handleCreate} style={{ width: '100%' }}>+ Nova Página</button>
                </div>

                <ul style={{ listStyle: 'none', padding: 0, overflowY: 'auto', flex: 1 }}>
                    {filteredPages.map(p => (
                        <li key={p.ID}
                            onClick={() => handleSelect(p)}
                            style={{
                                padding: '10px',
                                cursor: 'pointer',
                                background: selectedPage?.ID === p.ID ? 'var(--bg-tertiary)' : 'transparent',
                                borderRadius: '6px',
                                marginBottom: '4px',
                                whiteSpace: 'nowrap',
                                overflow: 'hidden',
                                textOverflow: 'ellipsis'
                            }}
                        >
                            {p.Title}
                        </li>
                    ))}
                </ul>
            </div>

            {/* Content Area */}
            <div className="wiki-content" style={{ flex: 1, padding: '20px', overflowY: 'auto', background: 'var(--bg-primary)' }}>
                {isEditing ? (
                    <div className="wiki-editor" style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
                        <input
                            value={editTitle}
                            onChange={e => setEditTitle(e.target.value)}
                            style={{ marginBottom: '15px', fontSize: '1.5rem', padding: '10px', fontWeight: 'bold' }}
                            placeholder="Título da Página"
                        />
                        <div style={{ flex: 1, marginBottom: '10px' }}>
                            <MDEditor
                                value={editContent}
                                onChange={setEditContent}
                                height={500}
                            />
                        </div>
                        <div style={{ marginTop: '10px', display: 'flex', justifyContent: 'flex-end', gap: '10px' }}>
                            <button onClick={handleSave}>Salvar</button>
                            <button onClick={() => setIsEditing(false)} style={{ background: 'var(--bg-secondary)' }}>Cancelar</button>
                        </div>
                    </div>
                ) : (
                    selectedPage ? (
                        <div className="wiki-viewer">
                            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '20px', borderBottom: '1px solid var(--border-color)', paddingBottom: '10px' }}>
                                <h1 style={{ margin: 0 }}>{selectedPage.Title}</h1>
                                <button onClick={handleEdit}>✏️ Editar</button>
                            </div>
                            <div style={{ padding: '0 10px' }}>
                                <MDEditor.Markdown source={selectedPage.Content} style={{ background: 'transparent', color: 'var(--text-primary)' }} />
                            </div>
                        </div>
                    ) : (
                        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100%', color: 'var(--text-secondary)' }}>
                            <div style={{ fontSize: '3rem', marginBottom: '10px' }}>📚</div>
                            <h3>Bem-vindo à Wiki do Canal</h3>
                            <p>Selecione uma página ou crie uma nova para começar.</p>
                        </div>
                    )
                )}
            </div>
        </div>
    );
}
