import { api, setUnauthorizedHandler } from './api';

// Who is logged in. The app shows the login page until someone is, and a
// setup page while there are no users at all.
export const auth = $state<{ state: 'checking' | 'in' | 'out' | 'setup' | 'error'; user: string; role: string; error: string }>({
  state: 'checking',
  user: '',
  role: '',
  error: '',
});

// admins can change alert rules, viewers can only look
export const isAdmin = () => auth.role === 'admin';

export async function checkSession() {
  try {
    const session = await api.session();
    if (session.user !== null) {
      auth.user = session.user;
      auth.role = session.role;
      auth.state = 'in';
    } else {
      auth.user = '';
      auth.state = session.hasUsers ? 'out' : 'setup';
    }
  } catch (e) {
    auth.error = (e as Error).message;
    auth.state = 'error';
  }
}

// a new session reloads the page, so every page starts from it
export async function logIn(user: string, password: string) {
  await api.login(user, password);
  window.location.reload();
}

export async function logOut() {
  try {
    await api.logout();
  } finally {
    window.location.reload();
  }
}

// a session that ends while the app is open, like one that expired, brings
// back the login page
setUnauthorizedHandler(() => {
  if (auth.state === 'in') auth.state = 'out';
});
