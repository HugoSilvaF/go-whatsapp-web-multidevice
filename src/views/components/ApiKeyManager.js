const SCOPE_OPTIONS = [
    { value: 'auth:manage', label: 'Auth management (/auth/keys*)' },
    { value: 'devices:manage', label: 'Device lifecycle routes' },
    { value: 'users:read', label: 'User/account read routes' },
    { value: 'chats:read', label: 'Chat read routes + websocket' },
    { value: 'messages:send', label: 'Send message/media routes' },
    { value: 'messages:manage', label: 'Message action routes' },
    { value: 'groups:manage', label: 'Group management routes' },
    { value: 'newsletters:manage', label: 'Newsletter routes' },
    { value: 'chatwoot:sync', label: 'Chatwoot sync routes' },
];

function parseScopesText(input) {
    if (!input) return [];
    return input
        .split(/[\n,]/)
        .map(s => s.trim().toLowerCase())
        .filter(Boolean);
}

export default {
    name: 'ApiKeyManager',
    data() {
        return {
            loading: false,
            createLoading: false,
            keys: [],
            includeRevoked: false,
            createForm: {
                name: '',
                expiresInDays: 30,
            },
            selectedScopes: {
                'messages:send': true,
            },
            customScopes: '',
            latestPlainKey: '',
            latestPlainKeyMeta: null,
            manualKeyValue: '',
            disableBasicBearerWhenApplying: true,
            scopeOptions: SCOPE_OPTIONS,
        };
    },
    computed: {
        mergedScopes() {
            const fromCheckboxes = Object.entries(this.selectedScopes)
                .filter(([, enabled]) => !!enabled)
                .map(([scope]) => scope);
            const fromCustom = parseScopesText(this.customScopes);
            return Array.from(new Set([...fromCheckboxes, ...fromCustom]));
        },
    },
    methods: {
        openModal() {
            $('#modalApiKeyManager').modal({
                onApprove: () => false,
            }).modal('show');
            this.fetchKeys();
        },
        async fetchKeys() {
            this.loading = true;
            try {
                const res = await window.http.get('/auth/keys', {
                    params: { include_revoked: this.includeRevoked },
                });
                this.keys = Array.isArray(res.data?.results) ? res.data.results : [];
            } catch (err) {
                showErrorInfo(err.response?.data?.message || err.message || 'Failed to load API keys');
            } finally {
                this.loading = false;
            }
        },
        async createKey() {
            if (this.createLoading) return;
            if (!this.createForm.name.trim()) {
                showErrorInfo('Key name is required');
                return;
            }
            if (this.mergedScopes.length === 0) {
                showErrorInfo('Select at least one scope');
                return;
            }

            this.createLoading = true;
            try {
                const payload = {
                    name: this.createForm.name.trim(),
                    scopes: this.mergedScopes,
                };
                const expires = Number(this.createForm.expiresInDays);
                if (Number.isFinite(expires) && expires > 0) {
                    payload.expires_in_days = expires;
                }
                const res = await window.http.post('/auth/keys', payload);
                const results = res.data?.results || {};
                this.latestPlainKey = results.api_key || '';
                this.latestPlainKeyMeta = {
                    id: results.id,
                    name: results.name,
                    scopes: results.scopes || [],
                    expires_at: results.expires_at || null,
                };
                showSuccessInfo('API key created');
                this.createForm.name = '';
                await this.fetchKeys();
            } catch (err) {
                showErrorInfo(err.response?.data?.message || err.message || 'Failed to create API key');
            } finally {
                this.createLoading = false;
            }
        },
        async rotateKey(id) {
            if (!id) return;
            if (!window.confirm('Rotate this key now? The old secret will stop working.')) return;
            try {
                const res = await window.http.post(`/auth/keys/${encodeURIComponent(id)}/rotate`);
                const results = res.data?.results || {};
                this.latestPlainKey = results.api_key || '';
                this.latestPlainKeyMeta = {
                    id: results.id,
                    name: results.name,
                    scopes: results.scopes || [],
                    expires_at: results.expires_at || null,
                };
                showSuccessInfo('API key rotated');
                await this.fetchKeys();
            } catch (err) {
                showErrorInfo(err.response?.data?.message || err.message || 'Failed to rotate key');
            }
        },
        async revokeKey(id) {
            if (!id) return;
            if (!window.confirm('Revoke this key? This cannot be undone.')) return;
            try {
                await window.http.delete(`/auth/keys/${encodeURIComponent(id)}`);
                showSuccessInfo('API key revoked');
                await this.fetchKeys();
            } catch (err) {
                showErrorInfo(err.response?.data?.message || err.message || 'Failed to revoke key');
            }
        },
        async copyValue(value) {
            if (!value) return;
            try {
                await navigator.clipboard.writeText(value);
                showSuccessInfo('Copied to clipboard');
            } catch (_err) {
                showErrorInfo('Clipboard unavailable. Copy manually.');
            }
        },
        applyApiKey(value) {
            const key = (value || '').trim();
            if (!key) {
                showErrorInfo('API key value is required');
                return;
            }
            window.http.defaults.headers.common['X-API-Key'] = key;
            if (this.disableBasicBearerWhenApplying) {
                delete window.http.defaults.headers.common['Authorization'];
            }
            showSuccessInfo('X-API-Key applied to UI requests');
        },
        clearApiKey() {
            delete window.http.defaults.headers.common['X-API-Key'];
            showSuccessInfo('X-API-Key removed from UI requests');
        },
        statusForKey(key) {
            if (key?.revoked_at) return 'revoked';
            if (key?.expires_at && new Date(key.expires_at) < new Date()) return 'expired';
            return 'active';
        },
        statusClass(key) {
            const status = this.statusForKey(key);
            if (status === 'active') return 'green';
            if (status === 'expired') return 'orange';
            return 'red';
        },
        formatDate(value) {
            if (!value) return '-';
            return moment(value).format('YYYY-MM-DD HH:mm:ss');
        },
    },
    template: `
    <div class="olive card" @click="openModal()" style="cursor: pointer">
        <div class="content">
            <a class="ui olive right ribbon label">Security</a>
            <div class="header">API Key Manager</div>
            <div class="description">
                Create, rotate, revoke, and apply scoped API keys.
            </div>
        </div>
    </div>

    <div class="ui fullscreen modal" id="modalApiKeyManager">
        <i class="close icon"></i>
        <div class="header">API Key Manager (Scoped)</div>
        <div class="content">
            <div class="ui stackable grid">
                <div class="six wide column">
                    <div class="ui segment">
                        <h4 class="ui header">Create key</h4>
                        <form class="ui form" @submit.prevent="createKey">
                            <div class="field">
                                <label>Name</label>
                                <input type="text" v-model="createForm.name" placeholder="e.g. n8n-prod">
                            </div>
                            <div class="field">
                                <label>Expires in days (optional)</label>
                                <input type="number" min="1" v-model.number="createForm.expiresInDays" placeholder="30">
                            </div>
                            <div class="field">
                                <label>Scopes</label>
                                <div class="ui relaxed list">
                                    <div class="item" v-for="scope in scopeOptions" :key="scope.value">
                                        <div class="ui checkbox">
                                            <input type="checkbox" :checked="selectedScopes[scope.value] === true"
                                                @change="selectedScopes[scope.value] = !selectedScopes[scope.value]">
                                            <label><code>{{ scope.value }}</code> - {{ scope.label }}</label>
                                        </div>
                                    </div>
                                </div>
                            </div>
                            <div class="field">
                                <label>Custom scopes (comma/new line)</label>
                                <textarea rows="2" v-model="customScopes" placeholder="custom:scope"></textarea>
                            </div>
                            <div class="ui small info message">
                                Final scopes: <code>{{ mergedScopes.join(', ') || '-' }}</code>
                            </div>
                            <button class="ui primary button" :class="{loading: createLoading}" type="submit">
                                Create key
                            </button>
                        </form>
                    </div>

                    <div class="ui segment">
                        <h4 class="ui header">Apply API key in UI</h4>
                        <div class="ui form">
                            <div class="field">
                                <label>X-API-Key value</label>
                                <input type="text" v-model="manualKeyValue" placeholder="gowa.<id>.<secret>">
                            </div>
                            <div class="field">
                                <div class="ui checkbox">
                                    <input type="checkbox" v-model="disableBasicBearerWhenApplying">
                                    <label>Remove Authorization header when applying API key</label>
                                </div>
                            </div>
                            <button class="ui button positive" @click="applyApiKey(manualKeyValue)">Apply key</button>
                            <button class="ui button" @click="clearApiKey">Clear key</button>
                        </div>
                    </div>
                </div>

                <div class="ten wide column">
                    <div class="ui segment">
                        <div class="ui clearing segment">
                            <h4 class="ui left floated header">Existing keys</h4>
                            <div class="ui right floated checkbox">
                                <input type="checkbox" v-model="includeRevoked" @change="fetchKeys">
                                <label>Include revoked</label>
                            </div>
                        </div>

                        <div v-if="loading" class="ui active centered inline loader"></div>
                        <table v-else class="ui celled compact striped table">
                            <thead>
                                <tr>
                                    <th>ID</th>
                                    <th>Name</th>
                                    <th>Scopes</th>
                                    <th>Status</th>
                                    <th>Expires</th>
                                    <th>Actions</th>
                                </tr>
                            </thead>
                            <tbody>
                                <tr v-for="key in keys" :key="key.id">
                                    <td><code>{{ key.id }}</code></td>
                                    <td>{{ key.name }}</td>
                                    <td><code>{{ (key.scopes || []).join(', ') }}</code></td>
                                    <td>
                                        <span class="ui tiny label" :class="statusClass(key)">{{ statusForKey(key) }}</span>
                                    </td>
                                    <td>{{ formatDate(key.expires_at) }}</td>
                                    <td>
                                        <button class="ui mini button" @click="rotateKey(key.id)">Rotate</button>
                                        <button class="ui mini red button" @click="revokeKey(key.id)" :disabled="!!key.revoked_at">Revoke</button>
                                    </td>
                                </tr>
                                <tr v-if="keys.length === 0">
                                    <td colspan="6" class="center aligned">No keys found.</td>
                                </tr>
                            </tbody>
                        </table>
                    </div>

                    <div class="ui warning message" v-if="latestPlainKey">
                        <div class="header">Secret shown once</div>
                        <p>Store this secret now. It cannot be retrieved again after this screen.</p>
                        <div class="ui form">
                            <div class="field">
                                <label>API key</label>
                                <input type="text" :value="latestPlainKey" readonly>
                            </div>
                        </div>
                        <button class="ui button small" @click="copyValue(latestPlainKey)">Copy key</button>
                        <button class="ui button small positive" @click="applyApiKey(latestPlainKey)">Apply in UI</button>
                        <div class="ui tiny message" v-if="latestPlainKeyMeta">
                            <div><b>ID:</b> <code>{{ latestPlainKeyMeta.id }}</code></div>
                            <div><b>Name:</b> {{ latestPlainKeyMeta.name }}</div>
                            <div><b>Scopes:</b> <code>{{ (latestPlainKeyMeta.scopes || []).join(', ') }}</code></div>
                        </div>
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
