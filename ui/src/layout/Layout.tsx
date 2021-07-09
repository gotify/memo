import {createMuiTheme, MuiThemeProvider, Theme, WithStyles, withStyles} from '@material-ui/core';
import CssBaseline from '@material-ui/core/CssBaseline';
import axios, {AxiosResponse} from 'axios';
import * as React from 'react';
import {HashRouter, Redirect, Route, Switch} from 'react-router-dom';
import Header from './Header';
import LoadingSpinner from '../common/LoadingSpinner';
import Navigation from './Navigation';
import ScrollUpButton from '../common/ScrollUpButton';
import SettingsDialog from '../common/SettingsDialog';
import SnackBarHandler from '../snack/SnackBarHandler';
import * as config from '../config';
import Applications from '../application/Applications';
import Clients from '../client/Clients';
import Plugins from '../plugin/Plugins';
import PluginDetailView from '../plugin/PluginDetailView';
import Login from '../user/Login';
import Messages from '../message/Messages';
import Users from '../user/Users';
import {observer} from 'mobx-react';
import {observable} from 'mobx';
import {inject, Stores} from '../inject';
import {ConnectionErrorBanner} from '../common/ConnectionErrorBanner';
import {IVersion} from '../types';

const styles = (theme: Theme) => ({
    content: {
        margin: '0 auto',
        marginTop: 64,
        padding: theme.spacing(4),
        width: '100%',
        [theme.breakpoints.down('xs')]: {
            marginTop: 0,
        },
    },
});

const localStorageThemeKey = 'gotify-theme';
type ThemeKey = 'dark' | 'light';
const themeMap: Record<ThemeKey, Theme> = {
    light: createMuiTheme({
        palette: {
            type: 'light',
        },
    }),
    dark: createMuiTheme({
        palette: {
            type: 'dark',
        },
    }),
};

const isThemeKey = (value: string | null): value is ThemeKey =>
    value === 'light' || value === 'dark';

@observer
class Layout extends React.Component<
    WithStyles<'content'> & Stores<'currentUser' | 'snackManager'>
> {
    private static defaultVersion = '0.0.0';

    @observable
    private currentTheme: ThemeKey = 'dark';
    @observable
    private showSettings = false;
    @observable
    private version = Layout.defaultVersion;
    @observable
    private navOpen = false;
    @observable
    private showRegister = true; //TODO https://github.com/gotify/server/pull/394#discussion_r650559205

    private setNavOpen(open: boolean) {
        this.navOpen = open;
    }

    public componentDidMount() {
        this.registration = true; //TODO https://github.com/gotify/server/pull/394#discussion_r650559205
        if (this.version === Layout.defaultVersion) {
            axios.get(config.get('url') + 'version').then((resp: AxiosResponse<IVersion>) => {
                this.version = resp.data.version;
            });
        }

        const localStorageTheme = window.localStorage.getItem(localStorageThemeKey);
        if (isThemeKey(localStorageTheme)) {
            this.currentTheme = localStorageTheme;
        } else {
            window.localStorage.setItem(localStorageThemeKey, this.currentTheme);
        }
    }

    public render() {
        const {version, showSettings, currentTheme, showRegister} = this;
        const {
            classes,
            currentUser: {
                loggedIn,
                authenticating,
                user: {name, admin},
                logout,
                tryReconnect,
                connectionErrorMessage,
            },
        } = this.props;
        const theme = themeMap[currentTheme];
        const loginRoute = () =>
            loggedIn ? <Redirect to="/" /> : <Login showRegister={showRegister} />;
        return (
            <MuiThemeProvider theme={theme}>
                <HashRouter>
                    <div>
                        {!connectionErrorMessage ? null : (
                            <ConnectionErrorBanner
                                height={64}
                                retry={() => tryReconnect()}
                                message={connectionErrorMessage}
                            />
                        )}
                        <div style={{display: 'flex', flexDirection: 'column'}}>
                            <CssBaseline />
                            <Header
                                style={{top: !connectionErrorMessage ? 0 : 64}}
                                admin={admin}
                                name={name}
                                version={version}
                                loggedIn={loggedIn}
                                toggleTheme={this.toggleTheme.bind(this)}
                                showSettings={() => (this.showSettings = true)}
                                logout={logout}
                                setNavOpen={this.setNavOpen.bind(this)}
                            />
                            <div style={{display: 'flex'}}>
                                <Navigation
                                    loggedIn={loggedIn}
                                    navOpen={this.navOpen}
                                    setNavOpen={this.setNavOpen.bind(this)}
                                />
                                <main className={classes.content}>
                                    <Switch>
                                        {authenticating ? (
                                            <Route path="/">
                                                <LoadingSpinner />
                                            </Route>
                                        ) : null}
                                        <Route exact path="/login" render={loginRoute} />
                                        {loggedIn ? null : <Redirect to="/login" />}
                                        <Route exact path="/" component={Messages} />
                                        <Route exact path="/messages/:id" component={Messages} />
                                        <Route
                                            exact
                                            path="/applications"
                                            component={Applications}
                                        />
                                        <Route exact path="/clients" component={Clients} />
                                        <Route exact path="/users" component={Users} />
                                        <Route exact path="/plugins" component={Plugins} />
                                        <Route
                                            exact
                                            path="/plugins/:id"
                                            component={PluginDetailView}
                                        />
                                    </Switch>
                                </main>
                            </div>
                            {showSettings && (
                                <SettingsDialog fClose={() => (this.showSettings = false)} />
                            )}
                            <ScrollUpButton />
                            <SnackBarHandler />
                        </div>
                    </div>
                </HashRouter>
            </MuiThemeProvider>
        );
    }

    private toggleTheme() {
        this.currentTheme = this.currentTheme === 'dark' ? 'light' : 'dark';
        localStorage.setItem(localStorageThemeKey, this.currentTheme);
    }
}

export default withStyles(styles, {withTheme: true})(inject('currentUser', 'snackManager')(Layout));
