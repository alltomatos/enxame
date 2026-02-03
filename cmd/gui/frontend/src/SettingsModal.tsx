import React, { useState, useEffect } from 'react';
import { ClipboardSetText } from "../wailsjs/runtime";
import { ExportIdentity, GetRole, GetGridStats } from "../wailsjs/go/main/App";
import './SettingsModal.css';

interface SettingsModalProps {
    visible: boolean;
    onClose: () => void;
    nodeID: string;
}

const SettingsModal: React.FC<SettingsModalProps> = ({ visible, onClose, nodeID }) => {
    const [role, setRole] = useState("Loading...");
    const [gridStats, setGridStats] = useState<any>({ jobs_processed: 0 });
    const [copyFeedback, setCopyFeedback] = useState("");

    useEffect(() => {
        if (visible) {
            loadData();
        }
    }, [visible]);

    const loadData = async () => {
        const r = await GetRole();
        setRole(r);
        const s = await GetGridStats();
        setGridStats(s);
    }

    const handleCopy = () => {
        ClipboardSetText(nodeID);
        setCopyFeedback("Copied!");
        setTimeout(() => setCopyFeedback(""), 2000);
    }

    const handleExport = async () => {
        const err = await ExportIdentity();
        if (err === "") {
            alert("Backup saved successfully!");
        } else if (err !== "Cancelled or Error") {
            alert("Error: " + err);
        }
    };

    if (!visible) return null;

    return (
        <div className="modal-overlay" onClick={onClose}>
            <div className="modal-content" onClick={e => e.stopPropagation()}>
                <div className="modal-header">
                    <h2>Settings & Profile</h2>
                    <button className="close-btn" onClick={onClose}>&times;</button>
                </div>

                <div className="modal-body">
                    {/* Profile Section */}
                    <div className="section">
                        <h3>Identity</h3>
                        <div className="field-group">
                            <label>Node ID</label>
                            <div className="id-display">
                                <code>{nodeID}</code>
                                <button className="small-btn" onClick={handleCopy}>
                                    {copyFeedback || "Copy"}
                                </button>
                            </div>
                        </div>
                        <div className="field-group">
                            <label>Global Role</label>
                            <span className="role-badge">{role}</span>
                        </div>
                    </div>

                    {/* Grid Stats */}
                    <div className="section">
                        <h3>Grid Contribution</h3>
                        <div className="stats-grid">
                            <div className="stat-card">
                                <h4>Jobs Processed</h4>
                                <p>{gridStats.jobs_processed || 0}</p>
                            </div>
                            <div className="stat-card">
                                <h4>Reputation</h4>
                                <p>Unknown</p>
                            </div>
                        </div>
                    </div>

                    {/* Danger Zone */}
                    <div className="section danger-zone">
                        <h3>Danger Zone</h3>
                        <p>Backup your private identity key. Do not share this file!</p>
                        <button className="danger-btn" onClick={handleExport}>
                            🔑 Export Identity Key
                        </button>
                    </div>
                </div>
            </div>
        </div>
    );
};

export default SettingsModal;
