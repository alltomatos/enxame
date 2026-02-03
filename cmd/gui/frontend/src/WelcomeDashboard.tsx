import React from 'react';

interface WelcomeDashboardProps {
    onConnectClick: () => void;
    status: 'idle' | 'connecting' | 'connected' | 'error';
    errorMsg?: string;
}

const WelcomeDashboard: React.FC<WelcomeDashboardProps> = ({ onConnectClick, status, errorMsg }) => {
    return (
        <div style={{
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            height: '100%',
            backgroundColor: '#1a1a1a',
            color: '#fff',
            padding: '2rem',
            textAlign: 'center',
            animation: 'fadeIn 1.2s ease-out'
        }}>
            <style>
                {`
                @keyframes fadeIn {
                    from { opacity: 0; transform: translateY(10px); }
                    to { opacity: 1; transform: translateY(0); }
                }
                @keyframes spin {
                    0% { transform: rotate(0deg); }
                    100% { transform: rotate(360deg); }
                }
                `}
            </style>

            <div style={{ marginBottom: '2rem' }}>
                <div style={{
                    width: '120px',
                    height: '120px',
                    backgroundColor: '#ffd700',
                    borderRadius: '24px',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    fontSize: '4rem',
                    boxShadow: '0 0 30px rgba(255, 215, 0, 0.3)'
                }}>
                    🐝
                </div>
            </div>

            <h1 style={{ fontSize: '2.5rem', marginBottom: '1rem', fontWeight: 'bold' }}>
                Bem-vindo ao Enxame P2P
            </h1>
            <p style={{ fontSize: '1.2rem', color: '#aaa', marginBottom: '3rem', maxWidth: '500px' }}>
                Conecte-se à rede descentralizada para começar a colaborar de forma segura e soberana.
            </p>

            <div style={{ position: 'relative' }}>
                <button
                    onClick={onConnectClick}
                    disabled={status === 'connecting' || status === 'connected'}
                    style={{
                        backgroundColor: status === 'error' ? '#ff4444' : '#ffd700',
                        color: '#000',
                        border: 'none',
                        padding: '1rem 3rem',
                        fontSize: '1.2rem',
                        fontWeight: 'bold',
                        borderRadius: '12px',
                        cursor: status === 'connecting' ? 'not-allowed' : 'pointer',
                        transition: 'all 0.2s ease',
                        display: 'flex',
                        alignItems: 'center',
                        gap: '12px',
                        boxShadow: '0 4px 15px rgba(0,0,0,0.2)'
                    }}
                >
                    {status === 'connecting' && (
                        <div style={{
                            width: '20px',
                            height: '20px',
                            border: '3px solid rgba(0,0,0,0.1)',
                            borderTop: '3px solid #000',
                            borderRadius: '50%',
                            animation: 'spin 1s linear infinite'
                        }} />
                    )}
                    {status === 'idle' && 'Conectar à Comunidade'}
                    {status === 'connecting' && 'Buscando satélites...'}
                    {status === 'connected' && 'Conectado'}
                    {status === 'error' && 'Tentar Novamente'}
                </button>
            </div>

            {status === 'error' && (
                <div style={{ marginTop: '1.5rem', color: '#ff4444', fontWeight: '500' }}>
                    🔴 {errorMsg || 'Erro na conexão. Verifique o servidor relay.'}
                </div>
            )}

            <div style={{ marginTop: 'auto', color: '#555', fontSize: '0.9rem' }}>
                Soberania Digital & Criptografia Ponta-a-Ponta
            </div>
        </div>
    );
};

export default WelcomeDashboard;
