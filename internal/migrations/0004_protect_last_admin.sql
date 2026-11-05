CREATE TRIGGER protect_last_admin_update
BEFORE UPDATE OF role, disabled_at, deleted_at, email_verified_at ON users
WHEN OLD.role = 'admin'
  AND OLD.disabled_at IS NULL AND OLD.deleted_at IS NULL AND OLD.email_verified_at IS NOT NULL
  AND (NEW.role != 'admin' OR NEW.disabled_at IS NOT NULL OR NEW.deleted_at IS NOT NULL OR NEW.email_verified_at IS NULL)
  AND NOT EXISTS (
      SELECT 1 FROM users
      WHERE id != OLD.id AND role = 'admin'
        AND disabled_at IS NULL AND deleted_at IS NULL AND email_verified_at IS NOT NULL
  )
BEGIN
  SELECT RAISE(ABORT, 'cannot remove last active administrator');
END;

CREATE TRIGGER protect_last_admin_delete
BEFORE DELETE ON users
WHEN OLD.role = 'admin'
  AND OLD.disabled_at IS NULL AND OLD.deleted_at IS NULL AND OLD.email_verified_at IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM users
      WHERE id != OLD.id AND role = 'admin'
        AND disabled_at IS NULL AND deleted_at IS NULL AND email_verified_at IS NOT NULL
  )
BEGIN
  SELECT RAISE(ABORT, 'cannot delete last active administrator');
END;
