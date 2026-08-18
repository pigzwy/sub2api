/**
 * 对象存储卡片把「异步生图存储」和「异步媒体存储」两份后端配置渲染成一张卡。
 *
 * 后端仍是两个独立的 settings key（image_storage_config / video_storage_config），
 * 没有迁移。共用的是**凭证**而不是存储桶：同一个 R2/S3 账号下给图片、视频各开一个
 * 桶是最常见的用法，把桶塞进共用块只会逼出第二套完整的凭证表单。
 *
 * 因此卡片的分组是：顶部一套凭证，下面每种类型各自的桶与 Key 前缀。
 * 音频没有自己的桶——它和视频同属 video_storage_config，只能有独立前缀。
 */

/** 两份配置共有的凭证字段（Secret 永不回传，故不参与比较）。 */
export interface StorageCredentials {
  reuse_backup_s3: boolean
  endpoint: string
  region: string
  access_key_id: string
  force_path_style: boolean
}

const trim = (value: string | undefined): string => (value ?? '').trim()

/** region 留空等价于 auto（后端 newS3Client 的默认值）。 */
const normalizedRegion = (value: string | undefined): string => trim(value) || 'auto'

/**
 * 判断两份配置是否使用同一套凭证。存储桶不参与比较——它是每种类型自己的字段。
 *
 * Secret 不参与比较：接口从不回传它，只能靠可见字段判断。因此这里刻意保守，任何
 * 一项对不上就算作不同凭证，宁可多展开一个凭证块，也不能悄悄把图片改指到另一个账号。
 */
export function sameStorageCredentials(a: StorageCredentials, b: StorageCredentials): boolean {
  if (a.reuse_backup_s3 !== b.reuse_backup_s3) return false
  // 复用备份凭证时，端点与密钥都来自备份卡，无需再比。
  if (a.reuse_backup_s3) return true
  return (
    trim(a.endpoint) === trim(b.endpoint) &&
    normalizedRegion(a.region) === normalizedRegion(b.region) &&
    trim(a.access_key_id) === trim(b.access_key_id) &&
    a.force_path_style === b.force_path_style
  )
}

/**
 * 该配置是否被真正配置过。全新安装时图片配置是空的，应当直接跟随共用凭证，而不是
 * 因为默认值不同就被判成「独立凭证」。
 */
export function storageCredentialsConfigured(
  credentials: StorageCredentials,
  secretConfigured: boolean,
): boolean {
  return secretConfigured || trim(credentials.endpoint) !== '' || trim(credentials.access_key_id) !== ''
}

/** 加载后判断图片是否需要保留独立的凭证块。 */
export function imageNeedsOwnCredentials(
  image: StorageCredentials,
  imageSecretConfigured: boolean,
  media: StorageCredentials,
): boolean {
  if (!storageCredentialsConfigured(image, imageSecretConfigured)) return false
  return !sameStorageCredentials(image, media)
}

/**
 * 把共用凭证复制到目标配置上。存储桶、前缀、公开域名等每种类型自己的字段原样保留。
 */
export function applyStorageCredentials<T extends StorageCredentials & { secret_access_key?: string }>(
  destination: T,
  source: StorageCredentials & { secret_access_key?: string },
): T {
  return {
    ...destination,
    reuse_backup_s3: source.reuse_backup_s3,
    endpoint: source.endpoint,
    region: source.region,
    access_key_id: source.access_key_id,
    force_path_style: source.force_path_style,
    // 留空表示沿用各自已存的 Secret，两个后端服务都是这个约定。
    secret_access_key: source.secret_access_key ?? '',
  }
}
