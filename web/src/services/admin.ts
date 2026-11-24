import { request } from '@umijs/max';

export type Envelope<T> = {
  code: number;
  message: string;
  data: T;
  page?: PageMeta;
};

export type PageMeta = {
  page: number;
  page_size: number;
  total: number;
  has_next: boolean;
};

export type ListParams = {
  page?: number;
  page_size?: number;
};

/**
 * 把 ProTable request 的 current/pageSize 映射为后端分页参数。
 * ProTable 首次加载不传页码时默认第一页、每页 20 条。
 */
export function pageParams(params: {
  current?: number;
  pageSize?: number;
}): ListParams {
  return {
    page: params.current ?? 1,
    page_size: params.pageSize ?? 20,
  };
}

/**
 * 把后端 Envelope 分页响应适配成 ProTable request 需要的返回结构。
 */
export async function toTableResult<T>(
  response: Envelope<T[]>,
): Promise<{ data: T[]; success: boolean; total: number }> {
  return {
    data: response.data,
    success: true,
    total: response.page?.total ?? 0,
  };
}

export type FileListParams = ListParams & {
  category_id?: number;
};

export type AppInfo = {
  name: string;
  version: string;
  time: string;
};

export type CapabilityStatus = {
  name: string;
  enabled: boolean;
  available: boolean;
  state: string;
  message?: string;
};

export type CapabilitiesResult = {
  capabilities: CapabilityStatus[];
  time: string;
};

export type LoginResult = {
  user: CurrentUser;
};

export type SetupState = {
  initialized: boolean;
};

export type SetupInput = {
  username: string;
  display_name: string;
  email?: string;
  password: string;
  site_name?: string;
};

export type AdminUser = {
  id: number;
  username: string;
  display_name: string;
  email: string;
  role_ids: number[];
  active_role_id: number;
  active: boolean;
  created_at: string;
  updated_at: string;
};

export type CurrentUser = {
  id: number;
  username: string;
  display_name: string;
  email: string;
  active_role_id: number;
  active_role: Role;
  default_path: string;
  roles: Role[];
  permissions: string[];
  menus: Menu[];
};

export type Role = {
  id: number;
  parent_id: number;
  code: string;
  name: string;
  permissions: string[];
  menu_ids: number[];
  api_ids: number[];
  button_ids: number[];
  data_role_ids: number[];
  default_path: string;
  active: boolean;
  created_at: string;
  updated_at: string;
};

export type PermissionDefinition = {
  token: string;
  resource: string;
  action: string;
  name: string;
};

export type Menu = {
  id: number;
  parent_id: number;
  name: string;
  path: string;
  icon: string;
  hidden: boolean;
  component: string;
  meta: MenuMeta;
  permission: string;
  sort: number;
  active: boolean;
  buttons: MenuButton[];
};

export type MenuMeta = {
  active_name: string;
  keep_alive: boolean;
  default_menu: boolean;
  close_tab: boolean;
  transition_type: string;
};

export type MenuButton = {
  id: number;
  menu_id: number;
  name: string;
  description: string;
  created_at: string;
  updated_at: string;
};

export type APIResource = {
  id: number;
  method: string;
  path: string;
  description: string;
  group: string;
  permission: string;
  created_at: string;
  updated_at: string;
};

export type APIToken = {
  id: number;
  admin_id: number;
  role_id: number;
  name: string;
  description: string;
  prefix: string;
  active: boolean;
  expires_at?: string | null;
  last_used_at?: string | null;
  created_at: string;
  updated_at: string;
};

export type APITokenCreateResult = {
  token: APIToken;
  secret: string;
};

export type AdminIDsResult = {
  admin_ids: number[];
};

export type SystemConfig = {
  key: string;
  name: string;
  value: string;
  public: boolean;
  updated_at: string;
};

export type SystemParam = {
  id: number;
  name: string;
  key: string;
  value: string;
  desc: string;
  created_at: string;
  updated_at: string;
};

export type SystemVersion = {
  id: number;
  version: string;
  name: string;
  description: string;
  published_at: string;
  created_at: string;
  updated_at: string;
};

export type VersionBundle = {
  version: VersionInfo;
  menus?: VersionMenu[];
  dictionaries?: VersionDictionary[];
};

export type VersionInfo = {
  name: string;
  code: string;
  description?: string;
  export_time?: string;
};

export type VersionMenu = {
  name: string;
  path: string;
  icon?: string;
  hidden?: boolean;
  component: string;
  meta?: VersionMenuMeta;
  permission?: string;
  sort?: number;
  active?: boolean;
  buttons?: VersionButton[];
  children?: VersionMenu[];
};

export type VersionMenuMeta = {
  active_name?: string;
  keep_alive?: boolean;
  default_menu?: boolean;
  close_tab?: boolean;
  transition_type?: string;
};

export type VersionButton = {
  name: string;
  description?: string;
};

export type VersionDictionary = {
  code: string;
  name: string;
  items?: VersionDictionaryItem[];
};

export type VersionDictionaryItem = {
  parent_id?: number;
  label: string;
  value: string;
  extend?: string;
  sort?: number;
  active?: boolean;
  level?: number;
  path?: string;
};

export type DictionaryBundle = {
  export_time?: string;
  dictionaries: VersionDictionary[];
};

export type Dictionary = {
  id: number;
  code: string;
  name: string;
  items: DictionaryItem[];
};

export type DictionaryItem = {
  id: number;
  parent_id: number;
  label: string;
  value: string;
  extend: string;
  sort: number;
  active: boolean;
  level: number;
  path: string;
  children: DictionaryItem[];
};

export type FileObject = {
  id: number;
  name: string;
  url: string;
  size: number;
  content_type: string;
  category_id: number;
  created_at: string;
};

export type FileCategory = {
  id: number;
  parent_id: number;
  name: string;
  children: FileCategory[];
  created_at: string;
  updated_at: string;
};

export type OperationLog = {
  id: number;
  actor_id: number;
  action: string;
  resource: string;
  resource_id: string;
  method: string;
  path: string;
  ip: string;
  user_agent: string;
  success: boolean;
  message: string;
  created_at: string;
};

export type LoginLog = {
  id: number;
  admin_id: number;
  username: string;
  ip: string;
  user_agent: string;
  success: boolean;
  reason: string;
  created_at: string;
};

export type SystemErrorLog = {
  id: number;
  code: number;
  message: string;
  detail: string;
  method: string;
  path: string;
  ip: string;
  user_agent: string;
  request_id: string;
  user_id: string;
  resolved: boolean;
  resolve_note: string;
  resolved_by: number;
  resolved_at?: string | null;
  created_at: string;
};

export type AdminCreateInput = {
  username: string;
  display_name: string;
  email?: string;
  password: string;
  role_ids: number[];
  active_role_id?: number;
  active: boolean;
};

export type AdminUpdateInput = {
  display_name?: string;
  email?: string;
  password?: string;
  role_ids?: number[];
  active_role_id?: number;
  active?: boolean;
};

export type RoleCreateInput = {
  parent_id: number;
  code: string;
  name: string;
  permissions: string[];
  menu_ids?: number[];
  api_ids?: number[];
  button_ids?: number[];
  data_role_ids?: number[];
  default_path?: string;
  active: boolean;
};

export type RoleUpdateInput = {
  parent_id?: number;
  name?: string;
  permissions?: string[];
  menu_ids?: number[];
  api_ids?: number[];
  button_ids?: number[];
  data_role_ids?: number[];
  default_path?: string;
  active?: boolean;
};

export type RoleCopyInput = {
  parent_id?: number;
  code: string;
  name: string;
  default_path?: string;
  active?: boolean;
};

export type MenuInput = {
  parent_id: number;
  name: string;
  path: string;
  icon?: string;
  hidden: boolean;
  component: string;
  meta: MenuMetaInput;
  permission?: string;
  sort: number;
  active: boolean;
  buttons?: MenuButtonInput[];
};

export type MenuMetaInput = {
  active_name?: string;
  keep_alive: boolean;
  default_menu: boolean;
  close_tab: boolean;
  transition_type?: string;
};

export type MenuButtonInput = {
  id?: number;
  name: string;
  description?: string;
};

export type APITokenInput = {
  admin_id?: number;
  role_id?: number;
  name: string;
  description?: string;
  active: boolean;
  days?: number;
  expires_at?: string | null;
};

export type ConfigInput = {
  name: string;
  value: string;
  public: boolean;
};

export type ParamInput = {
  name: string;
  key: string;
  value: string;
  desc?: string;
};

export type VersionInput = {
  version: string;
  name: string;
  description?: string;
  published_at?: string;
};

export type ExportVersionInput = {
  version: string;
  name: string;
  description?: string;
  menu_ids?: number[];
  dictionary_ids?: number[];
};

export type DictionaryCreateInput = {
  code: string;
  name: string;
};

export type DictionaryUpdateInput = {
  name: string;
};

export type DictionaryItemInput = {
  parent_id?: number;
  label: string;
  value: string;
  extend?: string;
  sort: number;
  active: boolean;
};

export async function appInfo(): Promise<AppInfo> {
  return request<AppInfo>('/api/info');
}

export async function capabilities(): Promise<CapabilitiesResult> {
  return request<CapabilitiesResult>('/api/capabilities');
}

export async function setupState(): Promise<SetupState> {
  const response = await request<Envelope<SetupState>>('/api/setup/state', {
    method: 'GET',
  });
  return response.data;
}

export async function submitSetup(body: SetupInput): Promise<SetupState> {
  const response = await request<Envelope<SetupState>>('/api/setup', {
    method: 'POST',
    data: body,
  });
  return response.data;
}

export async function login(body: {
  username: string;
  password: string;
}): Promise<LoginResult> {
  const response = await request<Envelope<LoginResult>>('/api/auth/login', {
    method: 'POST',
    data: body,
  });
  return response.data;
}

export async function logout(): Promise<void> {
  try {
    await request('/api/auth/logout', { method: 'POST' });
  } catch {
    // Local logout must still complete when the revocation request cannot
    // reach the server, otherwise the UI can trap an operator in a stale session.
  }
}

export async function currentUser(): Promise<CurrentUser> {
  const response = await request<Envelope<CurrentUser>>('/api/auth/me', {
    method: 'GET',
  });
  return response.data;
}

export async function updateCurrentUserProfile(body: {
  display_name: string;
  email?: string;
}): Promise<CurrentUser> {
  const response = await request<Envelope<CurrentUser>>('/api/auth/me', {
    method: 'PATCH',
    data: body,
  });
  return response.data;
}

export async function switchRole(roleID: number): Promise<LoginResult> {
  const response = await request<Envelope<LoginResult>>('/api/auth/role', {
    method: 'POST',
    data: { role_id: roleID },
  });
  return response.data;
}

export async function changePassword(body: {
  current_password: string;
  new_password: string;
}): Promise<void> {
  await request('/api/auth/password', {
    method: 'POST',
    data: body,
  });
}

export async function listAdmins(
  params?: ListParams,
): Promise<Envelope<AdminUser[]>> {
  return request<Envelope<AdminUser[]>>('/api/admins', { params });
}

export async function createAdmin(body: AdminCreateInput): Promise<void> {
  await request('/api/admins', { method: 'POST', data: body });
}

export async function updateAdmin(
  id: number,
  body: AdminUpdateInput,
): Promise<void> {
  await request(`/api/admins/${id}`, { method: 'PATCH', data: body });
}

export async function deleteAdmin(id: number): Promise<void> {
  await request(`/api/admins/${id}`, { method: 'DELETE' });
}

export async function listRoles(
  params?: ListParams,
): Promise<Envelope<Role[]>> {
  return request<Envelope<Role[]>>('/api/roles', { params });
}

export async function listPermissions(): Promise<PermissionDefinition[]> {
  const response =
    await request<Envelope<PermissionDefinition[]>>('/api/permissions');
  return response.data;
}

export async function createRole(body: RoleCreateInput): Promise<void> {
  await request('/api/roles', { method: 'POST', data: body });
}

export async function updateRole(
  id: number,
  body: RoleUpdateInput,
): Promise<void> {
  await request(`/api/roles/${id}`, { method: 'PATCH', data: body });
}

export async function deleteRole(id: number): Promise<void> {
  await request(`/api/roles/${id}`, { method: 'DELETE' });
}

export async function copyRole(id: number, body: RoleCopyInput): Promise<void> {
  await request(`/api/roles/${id}/copy`, { method: 'POST', data: body });
}

export async function listRoleAdmins(id: number): Promise<number[]> {
  const response = await request<Envelope<AdminIDsResult>>(
    `/api/roles/${id}/admins`,
  );
  return response.data.admin_ids;
}

export async function setRoleAdmins(
  id: number,
  adminIDs: number[],
): Promise<number[]> {
  const response = await request<Envelope<AdminIDsResult>>(
    `/api/roles/${id}/admins`,
    { method: 'PUT', data: { admin_ids: adminIDs } },
  );
  return response.data.admin_ids;
}

export async function listAPIs(
  params?: ListParams,
): Promise<Envelope<APIResource[]>> {
  return request<Envelope<APIResource[]>>('/api/apis', { params });
}

export async function readAPI(id: number): Promise<APIResource> {
  const response = await request<Envelope<APIResource>>(`/api/apis/${id}`);
  return response.data;
}

export async function listAPITokens(
  params?: ListParams,
): Promise<Envelope<APIToken[]>> {
  return request<Envelope<APIToken[]>>('/api/api-tokens', { params });
}

export async function createAPIToken(
  body: APITokenInput,
): Promise<APITokenCreateResult> {
  const response = await request<Envelope<APITokenCreateResult>>(
    '/api/api-tokens',
    { method: 'POST', data: body },
  );
  return response.data;
}

export async function updateAPIToken(
  id: number,
  body: APITokenInput,
): Promise<void> {
  await request(`/api/api-tokens/${id}`, { method: 'PATCH', data: body });
}

export async function deleteAPIToken(id: number): Promise<void> {
  await request(`/api/api-tokens/${id}`, { method: 'DELETE' });
}

export async function listMenus(): Promise<Menu[]> {
  const response = await request<Envelope<Menu[]>>('/api/menus');
  return response.data;
}

export async function readMenu(id: number): Promise<Menu> {
  const response = await request<Envelope<Menu>>(`/api/menus/${id}`);
  return response.data;
}

export async function createMenu(body: MenuInput): Promise<void> {
  await request('/api/menus', { method: 'POST', data: body });
}

export async function updateMenu(id: number, body: MenuInput): Promise<void> {
  await request(`/api/menus/${id}`, { method: 'PATCH', data: body });
}

export async function deleteMenu(id: number): Promise<void> {
  await request(`/api/menus/${id}`, { method: 'DELETE' });
}

export async function listConfigs(): Promise<SystemConfig[]> {
  const response = await request<Envelope<SystemConfig[]>>('/api/system/configs');
  return response.data;
}

export async function upsertConfig(key: string, body: ConfigInput): Promise<void> {
  await request(`/api/system/configs/${encodeURIComponent(key)}`, {
    method: 'PUT',
    data: body,
  });
}

export async function deleteConfig(key: string): Promise<void> {
  await request(`/api/system/configs/${encodeURIComponent(key)}`, {
    method: 'DELETE',
  });
}

export async function listParams(
  params?: ListParams & { name?: string; key?: string },
): Promise<Envelope<SystemParam[]>> {
  return request<Envelope<SystemParam[]>>('/api/system/params', { params });
}

export async function readParam(id: number): Promise<SystemParam> {
  const response = await request<Envelope<SystemParam>>(
    `/api/system/params/${id}`,
  );
  return response.data;
}

export async function readParamByKey(key: string): Promise<SystemParam> {
  const response = await request<Envelope<SystemParam>>(
    `/api/system/params/key/${encodeURIComponent(key)}`,
  );
  return response.data;
}

export async function createParam(body: ParamInput): Promise<void> {
  await request('/api/system/params', { method: 'POST', data: body });
}

export async function updateParam(
  id: number,
  body: ParamInput,
): Promise<void> {
  await request(`/api/system/params/${id}`, { method: 'PATCH', data: body });
}

export async function deleteParam(id: number): Promise<void> {
  await request(`/api/system/params/${id}`, { method: 'DELETE' });
}

export async function batchDeleteParams(ids: number[]): Promise<void> {
  await request('/api/system/params/batch-delete', {
    method: 'POST',
    data: { ids },
  });
}

export async function listVersions(): Promise<SystemVersion[]> {
  const response = await request<Envelope<SystemVersion[]>>(
    '/api/system/versions',
  );
  return response.data;
}

export async function readVersion(id: number): Promise<SystemVersion> {
  const response = await request<Envelope<SystemVersion>>(
    `/api/system/versions/${id}`,
  );
  return response.data;
}

export async function createVersion(body: VersionInput): Promise<void> {
  await request('/api/system/versions', { method: 'POST', data: body });
}

export async function exportVersion(
  body: ExportVersionInput,
): Promise<SystemVersion> {
  const response = await request<Envelope<SystemVersion>>(
    '/api/system/versions/export',
    { method: 'POST', data: body },
  );
  return response.data;
}

export async function importVersion(
  body: VersionBundle,
): Promise<SystemVersion> {
  const response = await request<Envelope<SystemVersion>>(
    '/api/system/versions/import',
    { method: 'POST', data: body },
  );
  return response.data;
}

export async function updateVersion(
  id: number,
  body: VersionInput,
): Promise<void> {
  await request(`/api/system/versions/${id}`, {
    method: 'PATCH',
    data: body,
  });
}

export async function deleteVersion(id: number): Promise<void> {
  await request(`/api/system/versions/${id}`, { method: 'DELETE' });
}

export async function batchDeleteVersions(ids: number[]): Promise<void> {
  await request('/api/system/versions/batch-delete', {
    method: 'POST',
    data: { ids },
  });
}

export async function downloadVersionJSON(id: number): Promise<Blob> {
  const response = await fetch(`/api/system/versions/${id}/download`, {
    credentials: 'include',
  });
  if (!response.ok) {
    throw new Error('download version json failed');
  }
  return response.blob();
}

export async function listDictionaries(): Promise<Dictionary[]> {
  const response = await request<Envelope<Dictionary[]>>('/api/dictionaries');
  return response.data;
}

export async function exportDictionaries(): Promise<Blob> {
  const response = await fetch('/api/dictionaries/export', {
    credentials: 'include',
  });
  if (!response.ok) {
    throw new Error('export dictionaries failed');
  }
  return response.blob();
}

export async function importDictionaries(
  body: DictionaryBundle,
): Promise<Dictionary[]> {
  const response = await request<Envelope<Dictionary[]>>(
    '/api/dictionaries/import',
    { method: 'POST', data: body },
  );
  return response.data;
}

export async function createDictionary(
  body: DictionaryCreateInput,
): Promise<void> {
  await request('/api/dictionaries', { method: 'POST', data: body });
}

export async function updateDictionary(
  code: string,
  body: DictionaryUpdateInput,
): Promise<void> {
  await request(`/api/dictionaries/${encodeURIComponent(code)}`, {
    method: 'PATCH',
    data: body,
  });
}

export async function deleteDictionary(code: string): Promise<void> {
  await request(`/api/dictionaries/${encodeURIComponent(code)}`, {
    method: 'DELETE',
  });
}

export async function addDictionaryItem(
  code: string,
  body: DictionaryItemInput,
): Promise<void> {
  await request(`/api/dictionaries/${encodeURIComponent(code)}/items`, {
    method: 'POST',
    data: body,
  });
}

export async function updateDictionaryItem(
  code: string,
  itemID: number,
  body: DictionaryItemInput,
): Promise<void> {
  await request(
    `/api/dictionaries/${encodeURIComponent(code)}/items/${itemID}`,
    { method: 'PATCH', data: body },
  );
}

export async function deleteDictionaryItem(
  code: string,
  itemID: number,
): Promise<void> {
  await request(
    `/api/dictionaries/${encodeURIComponent(code)}/items/${itemID}`,
    { method: 'DELETE' },
  );
}

export async function listFileCategories(): Promise<FileCategory[]> {
  const response =
    await request<Envelope<FileCategory[]>>('/api/file-categories');
  return response.data;
}

export async function createFileCategory(body: {
  name: string;
  parent_id?: number;
}): Promise<FileCategory> {
  const response = await request<Envelope<FileCategory>>(
    '/api/file-categories',
    {
      method: 'POST',
      data: body,
    },
  );
  return response.data;
}

export async function updateFileCategory(
  id: number,
  body: { name: string; parent_id?: number },
): Promise<FileCategory> {
  const response = await request<Envelope<FileCategory>>(
    `/api/file-categories/${id}`,
    {
      method: 'PATCH',
      data: body,
    },
  );
  return response.data;
}

export async function deleteFileCategory(id: number): Promise<void> {
  await request(`/api/file-categories/${id}`, { method: 'DELETE' });
}

export async function listFiles(
  params?: FileListParams,
): Promise<Envelope<FileObject[]>> {
  return request<Envelope<FileObject[]>>('/api/files', { params });
}

export async function uploadFile(
  file: File,
  categoryID?: number,
): Promise<FileObject> {
  const body = new FormData();
  body.append('file', file);
  if (categoryID) {
    body.append('category_id', String(categoryID));
  }
  const response = await request<Envelope<FileObject>>('/api/files', {
    method: 'POST',
    data: body,
  });
  return response.data;
}

export async function importFileURL(body: {
  name?: string;
  url: string;
  category_id?: number;
}): Promise<FileObject> {
  const response = await request<Envelope<FileObject>>('/api/files/import-url', {
    method: 'POST',
    data: body,
  });
  return response.data;
}

export async function renameFile(id: number, name: string): Promise<FileObject> {
  const response = await request<Envelope<FileObject>>(`/api/files/${id}/name`, {
    method: 'PATCH',
    data: { name },
  });
  return response.data;
}

export async function deleteFile(id: number): Promise<void> {
  await request(`/api/files/${id}`, { method: 'DELETE' });
}

export async function listOperationLogs(
  params?: ListParams,
): Promise<Envelope<OperationLog[]>> {
  return request<Envelope<OperationLog[]>>('/api/logs/operations', { params });
}

export async function readOperationLog(id: number): Promise<OperationLog> {
  const response = await request<Envelope<OperationLog>>(
    `/api/logs/operations/${id}`,
  );
  return response.data;
}

export async function deleteOperationLog(id: number): Promise<void> {
  await request(`/api/logs/operations/${id}`, { method: 'DELETE' });
}

export async function batchDeleteOperationLogs(ids: number[]): Promise<void> {
  await request('/api/logs/operations/batch-delete', {
    method: 'POST',
    data: { ids },
  });
}

export async function listLoginLogs(
  params?: ListParams,
): Promise<Envelope<LoginLog[]>> {
  return request<Envelope<LoginLog[]>>('/api/logs/logins', { params });
}

export async function readLoginLog(id: number): Promise<LoginLog> {
  const response = await request<Envelope<LoginLog>>(`/api/logs/logins/${id}`);
  return response.data;
}

export async function deleteLoginLog(id: number): Promise<void> {
  await request(`/api/logs/logins/${id}`, { method: 'DELETE' });
}

export async function batchDeleteLoginLogs(ids: number[]): Promise<void> {
  await request('/api/logs/logins/batch-delete', {
    method: 'POST',
    data: { ids },
  });
}

export async function listSystemErrorLogs(
  params?: ListParams,
): Promise<Envelope<SystemErrorLog[]>> {
  return request<Envelope<SystemErrorLog[]>>('/api/logs/errors', { params });
}

export async function readSystemErrorLog(id: number): Promise<SystemErrorLog> {
  const response = await request<Envelope<SystemErrorLog>>(
    `/api/logs/errors/${id}`,
  );
  return response.data;
}

export async function deleteSystemErrorLog(id: number): Promise<void> {
  await request(`/api/logs/errors/${id}`, { method: 'DELETE' });
}

export async function batchDeleteSystemErrorLogs(ids: number[]): Promise<void> {
  await request('/api/logs/errors/batch-delete', {
    method: 'POST',
    data: { ids },
  });
}

export async function resolveSystemErrorLog(
  id: number,
  note?: string,
): Promise<SystemErrorLog> {
  const response = await request<Envelope<SystemErrorLog>>(
    `/api/logs/errors/${id}/resolve`,
    { method: 'POST', data: { note } },
  );
  return response.data;
}

export async function reopenSystemErrorLog(id: number): Promise<SystemErrorLog> {
  const response = await request<Envelope<SystemErrorLog>>(
    `/api/logs/errors/${id}/resolve`,
    { method: 'DELETE' },
  );
  return response.data;
}


// D1 character and product-model management APIs.
export type Character = {
  id: number; code: string; name: string; persona: string; description: string;
  created_at: string; updated_at: string;
};
export type CharacterInput = {
  code?: string; name: string; persona: string; description?: string;
};
export type DeviceType = {
  id: number; code: string; name: string; description: string;
  character_id: number; has_camera: boolean; has_microphone: boolean; has_speaker: boolean;
  created_at: string; updated_at: string;
};
export type DeviceTypeInput = {
  code?: string; name: string; description?: string;
  character_id: number; has_camera: boolean; has_microphone: boolean; has_speaker: boolean;
};
export async function listCharacters(): Promise<Character[]> {
  const response = await request<Envelope<Character[]>>('/api/characters');
  return response.data;
}
export async function createCharacter(data: CharacterInput): Promise<Character> {
  const response = await request<Envelope<Character>>('/api/characters', { method: 'POST', data });
  return response.data;
}
export async function updateCharacter(id: number, data: CharacterInput): Promise<Character> {
  const response = await request<Envelope<Character>>('/api/characters/' + id, { method: 'PATCH', data });
  return response.data;
}
export async function deleteCharacter(id: number): Promise<void> {
  await request('/api/characters/' + id, { method: 'DELETE' });
}
export async function listDeviceTypes(): Promise<DeviceType[]> {
  const response = await request<Envelope<DeviceType[]>>('/api/device-types');
  return response.data;
}
export async function createDeviceType(data: DeviceTypeInput): Promise<DeviceType> {
  const response = await request<Envelope<DeviceType>>('/api/device-types', { method: 'POST', data });
  return response.data;
}
export async function updateDeviceType(id: number, data: DeviceTypeInput): Promise<DeviceType> {
  const response = await request<Envelope<DeviceType>>('/api/device-types/' + id, { method: 'PATCH', data });
  return response.data;
}
export async function deleteDeviceType(id: number): Promise<void> {
  await request('/api/device-types/' + id, { method: 'DELETE' });
}

export type CapabilityResourceKind = 'voice' | 'tool' | 'skill' | 'knowledge_base';
export type CapabilityResource = {
  id: number;
  kind: CapabilityResourceKind;
  code: string;
  name: string;
  description: string;
  provider: string;
  reference: string;
  tool_config?: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
};
export type CapabilityResourceInput = {
  kind?: CapabilityResourceKind;
  code?: string;
  name: string;
  description?: string;
  provider?: string;
  reference: string;
  tool_config?: string;
  knowledge_filename?: string;
  knowledge_text?: string;
  enabled: boolean;
};

export async function listCapabilityResources(kind: CapabilityResourceKind): Promise<CapabilityResource[]> {
  const response = await request<Envelope<CapabilityResource[]>>('/api/capability-resources', {
    params: { kind },
  });
  return response.data;
}
export async function createCapabilityResource(data: CapabilityResourceInput): Promise<CapabilityResource> {
  const response = await request<Envelope<CapabilityResource>>('/api/capability-resources', { method: 'POST', data, ...(data.knowledge_filename ? { timeout: 120000 } : {}) });
  return response.data;
}
export async function updateCapabilityResource(id: number, data: CapabilityResourceInput): Promise<CapabilityResource> {
  const response = await request<Envelope<CapabilityResource>>('/api/capability-resources/' + id, { method: 'PATCH', data, ...(data.knowledge_filename ? { timeout: 120000 } : {}) });
  return response.data;
}
export async function deleteCapabilityResource(id: number): Promise<void> {
  await request('/api/capability-resources/' + id, { method: 'DELETE' });
}

export type DeviceTypeCapabilityDraft = {
  device_type_id: number;
  voice_id: number;
  tool_ids: number[];
  skill_ids: number[];
  knowledge_base_ids: number[];
};
export type DeviceTypeCapabilityDraftInput = Omit<DeviceTypeCapabilityDraft, 'device_type_id'>;

export async function getDeviceTypeCapabilityDraft(id: number): Promise<DeviceTypeCapabilityDraft> {
  const response = await request<Envelope<DeviceTypeCapabilityDraft>>(
    `/api/device-types/${id}/capability-draft`,
  );
  return response.data;
}
export async function saveDeviceTypeCapabilityDraft(
  id: number, data: DeviceTypeCapabilityDraftInput,
): Promise<DeviceTypeCapabilityDraft> {
  const response = await request<Envelope<DeviceTypeCapabilityDraft>>(
    `/api/device-types/${id}/capability-draft`,
    { method: 'PUT', data },
  );
  return response.data;
}

export type ActiveDeviceConfig = {
  version: number;
  published_by: number;
  published_at: string;
  device_type: DeviceType;
  character: Character;
  capabilities: DeviceTypeCapabilityDraft;
  voice?: CapabilityResource;
  tools: CapabilityResource[];
  skills: CapabilityResource[];
  knowledge_bases: CapabilityResource[];
};

// This API never reads the editable draft. The Agent Demo will use the same
// confirmed configuration boundary.
export async function getActiveDeviceConfig(id: number): Promise<ActiveDeviceConfig> {
  const response = await request<Envelope<ActiveDeviceConfig>>(
    `/api/device-types/${id}/active-config`,
  );
  return response.data;
}

export async function applyDeviceTypeConfig(id: number): Promise<ActiveDeviceConfig> {
  const response = await request<Envelope<ActiveDeviceConfig>>(
    `/api/device-types/${id}/active-config`,
    { method: 'PUT' },
  );
  return response.data;
}

export type DemoChatTurn = {
  role: 'user' | 'assistant';
  content: string;
};
export type DemoBehaviorEvent = {
  kind: 'action' | 'expression';
  code: string;
};

export type DemoChatResponse = {
  device_type_id: number;
  character_name: string;
  reply: string;
  memory_queued?: boolean;
  behaviors?: DemoBehaviorEvent[];
};

export async function chatWithCharacter(
  deviceTypeId: number, message: string, history: DemoChatTurn[],
  signal?: AbortSignal, remember = false, sessionId = '',
): Promise<DemoChatResponse> {
  const response = await request<Envelope<DemoChatResponse>>('/api/demo/chat', {
    method: 'POST',
    data: {
      device_type_id: deviceTypeId, message, history,
      ...(remember ? { remember: true, session_id: sessionId } : {}),
    },
    signal,
  });
  return response.data;
}


// Voice routes use the existing authenticated session and CSRF token.
// The browser uploads native Opus, not PCM disguised as compressed audio.
export async function recognizeDemoSpeech(opus: Blob, signal?: AbortSignal): Promise<string> {
  if (!['audio/ogg;codecs=opus', 'audio/webm;codecs=opus'].includes(opus.type)) {
    throw new Error('Expected Ogg or WebM Opus audio');
  }
  const { getCSRFToken } = await import('./csrf-token');
  const response = await fetch('/api/demo/asr', {
    method: 'POST',
    credentials: 'include',
    headers: {
      'Content-Type': opus.type,
      'X-CSRF-Token': getCSRFToken(),
    },
    body: opus,
    signal,
  });
  if (!response.ok) throw new Error('ASR request failed');
  const payload: Envelope<{ text: string }> = await response.json();
  if (payload.code !== 100001 || !payload.data?.text) throw new Error('ASR returned no transcript');
  return payload.data.text;
}

export async function synthesizeDemoSpeech(
  deviceTypeId: number, text: string, signal?: AbortSignal,
): Promise<Blob> {
  const canPlayOpus = document.createElement('audio')
    .canPlayType('audio/ogg; codecs="opus"') !== '';
  const { getCSRFToken } = await import('./csrf-token');
  const response = await fetch('/api/demo/tts', {
    method: 'POST',
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      Accept: canPlayOpus ? 'audio/ogg, audio/mpeg;q=0.8' : 'audio/mpeg',
      'X-CSRF-Token': getCSRFToken(),
    },
    body: JSON.stringify({ device_type_id: deviceTypeId, text }),
    signal,
  });
  const contentType = response.headers.get('Content-Type') ?? '';
  if (!response.ok || !(contentType.includes('audio/ogg') || contentType.includes('audio/mpeg'))) {
    throw new Error('TTS request failed');
  }
  const audio = await response.blob();
  if (!audio.size) throw new Error('TTS returned empty audio');
  return audio;
}

export async function streamCharacterSpeech(
  deviceTypeId: number, message: string, history: DemoChatTurn[],
  onAudio: (chunk: Uint8Array) => void, signal: AbortSignal,
  remember = false, sessionId = '',
): Promise<DemoChatResponse> {
  const { getCSRFToken } = await import('./csrf-token');
  const response = await fetch('/api/demo/chat', {
    method: 'POST',
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      Accept: 'application/x-ndjson',
      'X-CSRF-Token': getCSRFToken(),
    },
    body: JSON.stringify({
      device_type_id: deviceTypeId, message, history,
      ...(remember ? { remember: true, session_id: sessionId } : {}),
    }),
    signal,
  });
  if (!response.ok || !response.body || !response.headers.get('Content-Type')?.includes('application/x-ndjson')) {
    throw new Error('Streaming voice request failed');
  }
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let pending = '';
  let reply: DemoChatResponse | undefined;
  const handleLine = (line: string) => {
    if (!line.trim()) return;
    const event = JSON.parse(line);
    if (reply) throw new Error('Unexpected data after voice reply');
    if (event.type === 'audio' && event.mime === 'audio/ogg' && typeof event.audio === 'string') {
      onAudio(Uint8Array.from(atob(event.audio), (char) => char.charCodeAt(0)));
    } else if (event.type === 'done' && typeof event.reply?.reply === 'string') {
      reply = event.reply;
    } else {
      throw new Error('Invalid or failed voice stream');
    }
  };
  try {
    for (;;) {
      const { value, done } = await reader.read();
      if (done) break;
      pending += decoder.decode(value, { stream: true });
      const lines = pending.split('\n');
      pending = lines.pop() ?? '';
      for (const line of lines) handleLine(line);
    }
    pending += decoder.decode();
    handleLine(pending);
    if (!reply) throw new Error('Incomplete voice stream');
    return reply;
  } finally {
    await reader.cancel().catch(() => {});
    reader.releaseLock();
  }
}

export type DemoASRSession = {
  send(chunk: Blob): void;
  finish(): Promise<string>;
  cancel(): void;
};

// A same-origin WebSocket allows incremental microphone upload in HTTP/1.1
// development and production browsers. Only a configured non-Opus ASR
// provider selects the existing batch path; connection failures are errors.
export async function openDemoASR(mime: string, signal: AbortSignal): Promise<DemoASRSession | null> {
  if (signal.aborted) throw new Error('ASR canceled');
  const url = new URL('/api/demo/asr/live', window.location.href);
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
  url.searchParams.set('format', mime.startsWith('audio/ogg') ? 'ogg' : 'webm');
  const socket = new WebSocket(url);
  let ready = false;
  let finishing = false;
  let settleReady!: (value: boolean) => void;
  let rejectReady!: (error: Error) => void;
  let settleResult!: (text: string) => void;
  let rejectResult!: (error: Error) => void;
  const opened = new Promise<boolean>((resolve, reject) => { settleReady = resolve; rejectReady = reject; });
  const result = new Promise<string>((resolve, reject) => { settleResult = resolve; rejectResult = reject; });
  void result.catch(() => undefined);
  const cleanup = () => {
    window.clearTimeout(timer);
    signal.removeEventListener('abort', cancel);
  };
  const fail = (error: Error) => {
    cleanup();
    rejectReady(error);
    rejectResult(error);
    socket.close();
  };
  const cancel = () => fail(new Error('ASR canceled'));
  const timer = window.setTimeout(() => fail(new Error('ASR session timed out')), 45000);
  signal.addEventListener('abort', cancel, { once: true });
  socket.onerror = () => fail(new Error('ASR connection failed'));
  socket.onclose = () => fail(new Error('ASR connection closed'));
  socket.onmessage = ({ data }) => {
    try {
      const event = JSON.parse(data);
      if (!ready && event.type === 'batch') {
        cleanup();
        settleReady(false);
        socket.close();
      } else if (!ready && event.type === 'ready') {
        ready = true;
        settleReady(true);
      } else if (ready && finishing && event.type === 'done' && typeof event.text === 'string' && event.text) {
        cleanup();
        settleResult(event.text);
        socket.close();
      } else {
        fail(new Error('Live ASR failed'));
      }
    } catch {
      fail(new Error('Invalid ASR response'));
    }
  };
  if (signal.aborted) cancel();
  if (!await opened) return null;
  return {
    send(chunk) {
      if (finishing || socket.readyState !== WebSocket.OPEN || socket.bufferedAmount + chunk.size > (1 << 20)) {
        throw new Error('ASR upload unavailable');
      }
      socket.send(chunk);
    },
    finish() {
      if (!finishing) {
        finishing = true;
        if (socket.readyState === WebSocket.OPEN) socket.send('end');
        else fail(new Error('ASR connection closed'));
      }
      return result;
    },
    cancel,
  };
}
