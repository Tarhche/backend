type UnAuthenticated = {
  status: "unauthenticated";
};

type Authenticated = {
  status: "authenticated";
  profile: {
    avatar: string;
    email: string;
    name: string;
    username: string;
    uuid: string;
  };
};

export type AuthState = Authenticated | UnAuthenticated;
