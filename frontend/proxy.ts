import { NextResponse, type NextRequest } from 'next/server';

/**
 * Cheap gate for the admin UI: no `df_admin` cookie → /admin/login?next=…
 * (Real validation happens in the API: a 401 makes the client redirect to the login page.)
 * The matcher deliberately excludes /admin-api/* so upload bodies are never buffered here.
 */
export function proxy(request: NextRequest) {
  const { pathname, search } = request.nextUrl;
  if (pathname === '/admin/login' || pathname.startsWith('/admin/login/')) return NextResponse.next();
  if (request.cookies.has('df_admin')) return NextResponse.next();
  const url = request.nextUrl.clone();
  url.pathname = '/admin/login';
  url.search = '';
  url.searchParams.set('next', pathname + search);
  return NextResponse.redirect(url);
}

export const config = {
  matcher: ['/admin', '/admin/:path*'],
};
