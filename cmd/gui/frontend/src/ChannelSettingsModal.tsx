import { useState } from 'react';
import { UpdateChannelAvatar } from '../wailsjs/go/main/App';

interface Props {
    visible: boolean;
    onClose: () => void;
    channelID: string;
}

export default function ChannelSettingsModal({ visible, onClose, channelID }: Props) {
    const [avatar, setAvatar] = useState("");
    const [isSaving, setIsSaving] = useState(false);

    if (!visible) return null;

    const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        const file = e.target.files?.[0];
        if (!file) return;

        const reader = new FileReader();
        reader.onload = (event) => {
            const dataUrl = event.target?.result as string;
            // Simplificação: redimensionamento automático poderia ser feito via Canvas aqui
            // No momento usamos a dataURL base64 diretamente
            setAvatar(dataUrl);
        };
        reader.readAsDataURL(file);
    };

    const handleSave = async () => {
        if (!avatar) return;
        setIsSaving(true);
        try {
            const err = await UpdateChannelAvatar(channelID, avatar);
            if (err) {
                alert("Erro ao atualizar avatar: " + err);
            } else {
                onClose();
            }
        } catch (e: any) {
            alert("Erro: " + e.toString());
        } finally {
            setIsSaving(false);
        }
    };

    return (
        <div className="modal-overlay">
            <div className="modal-content" style={{ maxWidth: '400px' }}>
                <h3>Configurações de {channelID}</h3>
                <div style={{ margin: '20px 0', textAlign: 'center' }}>
                    <div style={{
                        width: '120px',
                        height: '120px',
                        borderRadius: '50%',
                        background: '#333',
                        margin: '0 auto 15px',
                        overflow: 'hidden',
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        border: '2px solid var(--accent-primary)'
                    }}>
                        {avatar ? (
                            <img src={avatar} style={{ width: '100%', height: '100%', objectFit: 'cover' }} alt="Preview" />
                        ) : (
                            <span style={{ fontSize: '2rem' }}>🖼️</span>
                        )}
                    </div>

                    <input
                        type="file"
                        accept="image/*"
                        onChange={handleFileChange}
                        style={{ display: 'none' }}
                        id="avatar-upload"
                    />
                    <label
                        htmlFor="avatar-upload"
                        className="nav-item active"
                        style={{ cursor: 'pointer', display: 'inline-block', padding: '8px 15px' }}
                    >
                        Escolher Imagem
                    </label>
                </div>

                <div className="modal-actions" style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px' }}>
                    <button onClick={onClose} className="cancel-btn">Cancelar</button>
                    <button onClick={handleSave} className="send-btn" disabled={isSaving || !avatar}>
                        {isSaving ? "Salvando..." : "Salvar Avatar"}
                    </button>
                </div>
            </div>
        </div>
    );
}
