function makeSecret(length = 48) {
    const charset = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_';
    const bytes = new Uint8Array(length);
    crypto.getRandomValues(bytes);
    let out = '';
    for (let i = 0; i < bytes.length; i++) {
        out += charset[bytes[i] % charset.length];
    }
    return out;
}

export default {
    name: 'SecurityHardening',
    data() {
        return {
            generatedSecret: '',
            prodOrigins: 'https://app.example.com',
            rateLimitMax: 120,
            rateLimitWindowSec: 60,
        };
    },
    computed: {
        currentAuthorization() {
            return window.http?.defaults?.headers?.common?.Authorization || '(not set)';
        },
        currentApiKey() {
            return window.http?.defaults?.headers?.common?.['X-API-Key'] || '(not set)';
        },
        envSnippet() {
            return [
                `APP_SECURITY_HEADERS=true`,
                `APP_RATE_LIMIT_ENABLED=true`,
                `APP_RATE_LIMIT_MAX=${this.rateLimitMax}`,
                `APP_RATE_LIMIT_WINDOW_SEC=${this.rateLimitWindowSec}`,
                `APP_CORS_ORIGINS=${this.prodOrigins}`,
                `CHATWOOT_WEBHOOK_TOKEN=${this.generatedSecret || '<set-random-secret>'}`,
                `CHATWOOT_SYNC_INCLUDE_STATUS=false`,
                `CHATWOOT_SYNC_MAX_MEDIA_FILE_SIZE=10000000`,
                `WHATSAPP_AUTO_DOWNLOAD_STATUS_MEDIA=false`,
                `WHATSAPP_HISTORY_SYNC_DUMP_ENABLED=false`,
            ].join('\n');
        },
    },
    methods: {
        openModal() {
            $('#modalSecurityHardening').modal({
                onApprove: () => false,
            }).modal('show');
        },
        async copyValue(value) {
            try {
                await navigator.clipboard.writeText(value);
                showSuccessInfo('Copied to clipboard');
            } catch (_err) {
                showErrorInfo('Clipboard unavailable. Copy manually.');
            }
        },
        generateSecret() {
            this.generatedSecret = makeSecret(48);
        },
    },
    template: `
    <div class="teal card" @click="openModal()" style="cursor: pointer">
        <div class="content">
            <a class="ui teal right ribbon label">Security</a>
            <div class="header">Hardening Guide</div>
            <div class="description">
                Operational checklist and secure config snippets for production.
            </div>
        </div>
    </div>

    <div class="ui large modal" id="modalSecurityHardening">
        <i class="close icon"></i>
        <div class="header">Security And Performance Hardening</div>
        <div class="content">
            <div class="ui two column stackable grid">
                <div class="column">
                    <h4 class="ui header">Runtime auth context (UI client)</h4>
                    <div class="ui form">
                        <div class="field">
                            <label>Authorization header</label>
                            <input type="text" :value="currentAuthorization" readonly>
                        </div>
                        <div class="field">
                            <label>X-API-Key header</label>
                            <input type="text" :value="currentApiKey" readonly>
                        </div>
                    </div>

                    <h4 class="ui header" style="margin-top: 1.5rem;">Recommended production baseline</h4>
                    <div class="ui bulleted list">
                        <div class="item">Use scoped API keys (minimum scopes per integration).</div>
                        <div class="item">Rotate keys on schedule and after incidents.</div>
                        <div class="item">Enable rate limiting and monitor 429 behavior.</div>
                        <div class="item">Restrict CORS to trusted frontend origins only.</div>
                        <div class="item">Keep status/story media sync disabled unless required.</div>
                        <div class="item">Use webhook token validation for Chatwoot webhooks.</div>
                    </div>
                </div>

                <div class="column">
                    <h4 class="ui header">Generate Chatwoot webhook token</h4>
                    <button class="ui button" @click="generateSecret">Generate random token</button>
                    <div class="ui form" style="margin-top: 1rem;">
                        <div class="field">
                            <label>Generated token</label>
                            <input type="text" :value="generatedSecret || '(generate one)'" readonly>
                        </div>
                        <button class="ui mini button" :disabled="!generatedSecret" @click="copyValue(generatedSecret)">Copy token</button>
                    </div>

                    <h4 class="ui header" style="margin-top: 1.5rem;">.env hardening snippet</h4>
                    <div class="ui form">
                        <div class="field">
                            <label>Allowed CORS origins</label>
                            <input type="text" v-model="prodOrigins" placeholder="https://app.example.com">
                        </div>
                        <div class="two fields">
                            <div class="field">
                                <label>Rate limit max</label>
                                <input type="number" min="1" v-model.number="rateLimitMax">
                            </div>
                            <div class="field">
                                <label>Rate limit window sec</label>
                                <input type="number" min="1" v-model.number="rateLimitWindowSec">
                            </div>
                        </div>
                        <div class="field">
                            <textarea rows="12" :value="envSnippet" readonly></textarea>
                        </div>
                        <button class="ui button primary" @click="copyValue(envSnippet)">Copy .env snippet</button>
                    </div>
                </div>
            </div>
        </div>
        <div class="actions">
            <div class="ui approve button">Close</div>
        </div>
    </div>
    `,
};
