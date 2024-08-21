// This file was automatically @generated by gentypescript

type apiSchemaCollection = {
  "POST /ping": {
    Query: undefined;
    Request: undefined;
    Response: {
      message: string
    };
  };
  "GET /ping": {
    Query: undefined;
    Request: undefined;
    Response: {
      count: number
    };
  };
  "GET /ping/nested": {
    Query: undefined;
    Request: undefined;
    Response: {
      count: number
    };
  };
  "GET /echo": {
    Query: undefined;
    Request: {
      message: string
    };
    Response: {
      message?: string
    };
  };
  "GET /nested/now": {
    Query: undefined;
    Request: undefined;
    Response: {
      now: string
    };
  };
  "GET /nested/{epoch:[0-9]+}": {
    Query: undefined;
    Request: undefined;
    Response: {
      datetime: string
    };
  };
}

type method = keyof apiSchemaCollection extends `${infer M} ${string}` ? M : never;
type methodPathsByMethod<M extends method> = Extract<keyof apiSchemaCollection, `${M} ${string}`>
type pathByMethod<MP extends string> = MP extends `${method} ${infer P}` ? P : never
type pathsByMethod<M extends method> = pathByMethod<methodPathsByMethod<M>>

const hasApiRequest = <PM extends keyof apiSchemaCollection>(args: unknown): args is { data: apiSchemaCollection[PM]["Request"] } => {
  return !!(args as { data: unknown })?.data
}

const hasApiQuery = <PM extends keyof apiSchemaCollection>(args: unknown): args is { query: apiSchemaCollection[PM]["Query"] } => {
  return !!(args as { query: unknown })?.query
}
const apiPathBuilder = {
    "/nested/{epoch:[0-9]+}": (args: {epoch: string}) => `/nested/${args.epoch}`,
} as const

const hasApiPathBuilder = (path: string): path is keyof typeof apiPathBuilder => path in apiPathBuilder
type apiPathBuilderArgs<K extends keyof apiSchemaCollection> = K extends `${method} ${infer P}` ? (P extends keyof typeof apiPathBuilder ? Parameters<typeof apiPathBuilder[P]>[0] : never) : never

const hasApiPathArgs = <PM extends keyof apiSchemaCollection>(args: unknown): args is { pathArgs: apiPathBuilderArgs<PM> } => {
  return !!(args as { pathArgs: unknown })?.pathArgs
}

type pathCallArgs<PM extends keyof apiSchemaCollection> =
  (apiSchemaCollection[PM]["Request"] extends undefined ? {} : { data: apiSchemaCollection[PM]["Request"] }) &
  (apiPathBuilderArgs<PM> extends never ? {} : { pathArgs: apiPathBuilderArgs<PM> }) &
  (apiSchemaCollection[PM]["Query"] extends undefined ? {} : { query: apiSchemaCollection[PM]["Query"]})

type client = {
  post: <P extends pathsByMethod<"POST">>(path: P, args: pathCallArgs<`POST ${P}`>) => Promise<apiSchemaCollection[`POST ${P}`]["Response"]>
  get: <P extends pathsByMethod<"GET">>(path: P, args: pathCallArgs<`GET ${P}`>) => Promise<apiSchemaCollection[`GET ${P}`]["Response"]>
}

export const newClient = (baseURL: string = ""): client => {
  const fetchByPath = async <PM extends keyof apiSchemaCollection>(method: method, path: string, args: pathCallArgs<PM>) => {
    const builtPath = hasApiPathBuilder(path) && hasApiPathArgs(args) ? apiPathBuilder[path](args.pathArgs) : path
    const query = hasApiQuery(args) ? "?" + (new URLSearchParams(args.query).toString()) : ""
    const body = hasApiRequest(args) ? JSON.stringify(args.data) : undefined
    const response = await fetch(baseURL + builtPath + query, {
      method,
      headers: {
        "Content-Type": "application/json",
      },
      body,
    })

    if (!response.ok) {
      throw new Error(response.statusText)
    }
    return response.json() as Promise<apiSchemaCollection[PM]["Response"]>
  }
  const post = async <P extends pathsByMethod<"POST">>(path: P, args: pathCallArgs<`POST ${P}`>) => await fetchByPath("POST", path, args)
  const get = async <P extends pathsByMethod<"GET">>(path: P, args: pathCallArgs<`GET ${P}`>) => await fetchByPath("GET", path, args)

  return {
    post,
    get,
  }
}