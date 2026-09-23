package sirkel_errors

const (
	CodeOrganizationNameRequired Code = "organization_name_required"

	CodeOrganizationIDRequired Code = "organization_id_required"
	CodeEmailRequired          Code = "email_required"
	CodePasswordRequired       Code = "password_required"
	CodeNameRequired           Code = "name_required"
	CodeFirstSurnameRequired   Code = "first_surname_required"
	CodeSecondSurnameRequired  Code = "second_surname_required"

	CodeEmailAlreadyExists Code = "email_already_exists"
	CodePasswordHashFailed Code = "password_hash_failed"
	CodeUserCreateFailed   Code = "user_create_failed"

	CodeInvalidCredentials    Code = "invalid_credentials"
	CodeTokenGenerationFailed Code = "token_generation_failed"

	CodeInvalidLimit             Code = "invalid_limit"
	CodeInvalidOffset            Code = "invalid_offset"
	CodeOrganizationsIndexFailed Code = "organizations_index_failed"

	CodeOrganizationNotFound     Code = "organization_not_found"
	CodeOrganizationUpdateFailed Code = "organization_update_failed"

	CodeUserIDRequired   Code = "user_id_required"
	CodeNoFieldsToUpdate Code = "no_fields_to_update"
	CodeUserNotFound     Code = "user_not_found"
	CodeUserUpdateFailed Code = "user_update_failed"

	CodeUsersIndexFailed Code = "users_index_failed"
)
