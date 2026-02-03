
import React from 'react';

interface Member {
    NodeID: string;
    Role: string; // 'ADMIN', 'MODERATOR', 'MEMBER'
    IsOnline: boolean;
}

interface UserListProps {
    members: Member[];
    onUserClick: (nodeID: string) => void;
}

const UserList: React.FC<UserListProps> = ({ members, onUserClick }) => {

    // Helper to group members
    const admins = members.filter(m => m.Role === 'ADMIN' || m.Role === 'MODERATOR');
    const online = members.filter(m => m.IsOnline && m.Role !== 'ADMIN' && m.Role !== 'MODERATOR');
    const offline = members.filter(m => !m.IsOnline && m.Role !== 'ADMIN' && m.Role !== 'MODERATOR');

    const renderMember = (m: Member) => (
        <div key={m.NodeID}
            className={`user-item ${!m.IsOnline ? 'offline' : ''}`}
            onClick={() => onUserClick(m.NodeID)}>
            <div className="user-avatar-placeholder">
                {m.NodeID.substring(0, 2).toUpperCase()}
            </div>
            <div className="user-info">
                <span className="user-name">
                    {m.NodeID.length > 10 ? m.NodeID.substring(0, 10) + '...' : m.NodeID}
                </span>
                {m.Role !== 'MEMBER' && (
                    <span className="user-badge">{m.Role === 'ADMIN' ? '👑' : '🛡️'}</span>
                )}
            </div>
        </div>
    );

    return (
        <div className="user-list-sidebar">
            <div className="user-list-header">
                <h3>Members</h3>
            </div>

            <div className="user-list-content">
                {admins.length > 0 && (
                    <div className="user-group">
                        <h4>Admin & Mods — {admins.length}</h4>
                        {admins.map(renderMember)}
                    </div>
                )}

                <div className="user-group">
                    <h4>Online — {online.length}</h4>
                    {online.map(renderMember)}
                </div>

                <div className="user-group">
                    <h4>Offline — {offline.length}</h4>
                    {offline.map(renderMember)}
                </div>
            </div>
        </div>
    );
};

export default UserList;
