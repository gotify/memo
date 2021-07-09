import axios, {AxiosError, AxiosResponse} from 'axios';
import * as config from './config';
import {Base64} from 'js-base64';
import {detect} from 'detect-browser';
import {SnackReporter} from './snack/SnackManager';
import {observable} from 'mobx';
import {IClient, IUser} from './types';

const tokenKey = 'gotify-login-key';

export class CurrentUser {
    private tokenCache: string | null = null;
    private reconnectTimeoutId: number | null = null;
    private reconnectTime = 7500;
    @observable
    public loggedIn = false;
    @observable
    public authenticating = false;
    @observable
    public user: IUser = {name: 'unknown', admin: false, id: -1};
    @observable
    public connectionErrorMessage: string | null = null;

    public constructor(private readonly snack: SnackReporter) {}

    public token = (): string => {
        if (this.tokenCache !== null) {
            return this.tokenCache;
        }

        const localStorageToken = window.localStorage.getItem(tokenKey);
        if (localStorageToken) {
            this.tokenCache = localStorageToken;
            return localStorageToken;
        }

        return '';
    };

    private readonly setToken = (token: string) => {
        this.tokenCache = token;
        window.localStorage.setItem(tokenKey, token);
    };

    public register = async (name: string, pass: string) => {
        const f = false;
        axios
            .post(config.get('url') + 'user', {name, pass, f})
            .then(() => {
                this.snack('User Created. Logging in...');
                this.login(name, pass);
            })
            .catch(() => true); //errors are already handled by axios snack bar so need this to avoid crash
    };

    public login = async (username: string, password: string) => {
        this.loggedIn = false;
        this.authenticating = true;
        const browser = detect();
        const name = (browser && browser.name + ' ' + browser.version) || 'unknown browser';
        axios
            .create()
            .request({
                url: config.get('url') + 'client',
                method: 'POST',
                data: {name},
                // eslint-disable-next-line @typescript-eslint/naming-convention
                headers: {Authorization: 'Basic ' + Base64.encode(username + ':' + password)},
            })
            .then((resp: AxiosResponse<IClient>) => {
                this.snack(`A client named '${name}' was created for your session.`);
                this.setToken(resp.data.token);
                this.tryAuthenticate()
                    .then(() => {
                        this.authenticating = false;
                        this.loggedIn = true;
                    })
                    .catch(() => {
                        this.authenticating = false;
                        console.log(
                            'create client succeeded, but authenticated with given token failed'
                        );
                    });
            })
            .catch(() => {
                this.authenticating = false;
                return this.snack('Login failed');
            });
    };

    public tryAuthenticate = async (): Promise<AxiosResponse<IUser>> => {
        if (this.token() === '') {
            return Promise.reject();
        }

        return (
            axios
                .create()
                // eslint-disable-next-line @typescript-eslint/naming-convention
                .get(config.get('url') + 'current/user', {headers: {'X-Gotify-Key': this.token()}})
                .then((passThrough) => {
                    this.user = passThrough.data;
                    this.loggedIn = true;
                    this.connectionErrorMessage = null;
                    this.reconnectTime = 7500;
                    return passThrough;
                })
                .catch((error: AxiosError) => {
                    if (!error || !error.response) {
                        this.connectionError('No network connection or server unavailable.');
                        return Promise.reject(error);
                    }

                    if (error.response.status >= 500) {
                        this.connectionError(
                            `${error.response.statusText} (code: ${error.response.status}).`
                        );
                        return Promise.reject(error);
                    }

                    this.connectionErrorMessage = null;

                    if (error.response.status >= 400 && error.response.status < 500) {
                        this.logout();
                    }
                    return Promise.reject(error);
                })
        );
    };

    public logout = async () => {
        await axios
            .get(config.get('url') + 'client')
            .then((resp: AxiosResponse<IClient[]>) => {
                resp.data
                    .filter((client) => client.token === this.tokenCache)
                    .forEach((client) => axios.delete(config.get('url') + 'client/' + client.id));
            })
            .catch(() => Promise.resolve());
        window.localStorage.removeItem(tokenKey);
        this.tokenCache = null;
        this.loggedIn = false;
    };

    public changePassword = (pass: string) => {
        axios
            .post(config.get('url') + 'current/user/password', {pass})
            .then(() => this.snack('Password changed'));
    };

    public tryReconnect = (quiet = false) => {
        this.tryAuthenticate().catch(() => {
            if (!quiet) {
                this.snack('Reconnect failed');
            }
        });
    };

    private readonly connectionError = (message: string) => {
        this.connectionErrorMessage = message;
        if (this.reconnectTimeoutId !== null) {
            window.clearTimeout(this.reconnectTimeoutId);
        }
        this.reconnectTimeoutId = window.setTimeout(
            () => this.tryReconnect(true),
            this.reconnectTime
        );
        this.reconnectTime = Math.min(this.reconnectTime * 2, 120000);
    };
}
