import {proxyGraphQL} from '../../../lib/graphqlProxy';
export async function POST(request:Request) { return proxyGraphQL(request); }
