package entities

// ODRL (Open Digital Rights Language) 2.2 vocabulary.
// See: https://www.w3.org/TR/odrl-vocab/

// ODRL Policy types define the nature of a policy.
const (
	// PolicyTypeSet is a general-purpose policy.
	PolicyTypeSet = "odrl:Set"
	// PolicyTypeOffer is a policy proposed by the assigner.
	PolicyTypeOffer = "odrl:Offer"
	// PolicyTypeAgreement is a policy agreed upon by both parties.
	PolicyTypeAgreement = "odrl:Agreement"
)

// ODRL predicates for linking policies, rules, and parties.
const (
	// PredicatePermission links an assignee to a target with a permitted action.
	PredicatePermission = "odrl:permission"
	// PredicateProhibition links an assignee to a target with a prohibited action.
	PredicateProhibition = "odrl:prohibition"
	// PredicateDuty links an assignee to a target with an obligated action.
	PredicateDuty = "odrl:duty"
	// PredicateAssignee links a policy to the party receiving rights/duties.
	PredicateAssignee = "odrl:assignee"
	// PredicateAssigner links a policy to the party granting rights/duties.
	PredicateAssigner = "odrl:assigner"
)

// ODRL actions define what can be done with a target asset.
// See: https://www.w3.org/TR/odrl-vocab/#actionConcepts
const (
	// ActionUse is the top-level action encompassing all usage.
	ActionUse = "odrl:use"
	// ActionRead permits reading/viewing a target.
	ActionRead = "odrl:read"
	// ActionModify permits modifying/updating a target.
	ActionModify = "odrl:modify"
	// ActionDelete permits deleting a target.
	ActionDelete = "odrl:delete"
	// ActionExecute permits executing/invoking a target.
	ActionExecute = "odrl:execute"
	// ActionAggregate permits combining a target with other assets.
	ActionAggregate = "odrl:aggregate"
	// ActionDistribute permits distributing/sharing a target.
	ActionDistribute = "odrl:distribute"
	// ActionTransfer permits transferring ownership of a target.
	ActionTransfer = "odrl:transfer"
)

// FOAF (Friend of a Friend) vocabulary for agent modeling.
// See: http://xmlns.com/foaf/0.1/

// FOAF agent types classify parties in the system.
const (
	// AgentTypePerson is a human agent.
	AgentTypePerson = "foaf:Person"
	// AgentTypeOrganization is an organizational agent.
	AgentTypeOrganization = "foaf:Organization"
	// AgentTypeGroup is a group of agents.
	AgentTypeGroup = "foaf:Group"
	// AgentTypeSoftwareAgent is an automated software agent.
	AgentTypeSoftwareAgent = "foaf:Agent"
)

// FOAF predicates for agent relationships.
const (
	// PredicateMember links an agent to a group.
	PredicateMember = "foaf:member"
)

// Schema.org vocabulary for authentication and identity.
// See: https://schema.org/
const (
	// PredicateCredential links an agent to a credential.
	PredicateCredential = "schema:credential"
	// PredicateProvider identifies the identity provider for a credential.
	PredicateProvider = "schema:provider"
	// PredicateIdentifier links a credential to a provider-specific user ID.
	PredicateIdentifier = "schema:identifier"
	// PredicateAuthenticator links a session to an account scope.
	PredicateAuthenticator = "schema:authenticator"
	// PredicateSession links an agent to an authenticated session.
	PredicateSession = "schema:session"
)

// W3C ORG (Organization Ontology) vocabulary for role modeling.
// See: https://www.w3.org/TR/vocab-org/

// ORG predicates for organizational relationships.
const (
	// PredicateHasRole links an agent to a role they currently hold.
	PredicateHasRole = "org:hasRole"
	// PredicateHadRole links an agent to a role they previously held (for revocation tracking).
	PredicateHadRole = "org:hadRole"
	// PredicateMemberOf links an agent to an organization.
	PredicateMemberOf = "org:memberOf"
	// PredicateHasMember links an account/organization to a member agent.
	PredicateHasMember = "org:hasMember"
	// PredicateHadMember links an account/organization to a former member (for removal tracking).
	PredicateHadMember = "org:hadMember"
)

// Well-known roles for account membership.
const (
	RoleOwner  = "owner"
	RoleMember = "member"
	RoleAdmin  = "admin"
)

// Built-in credential providers.
const (
	// ProviderPassword identifies username/password credentials. The
	// credential's provider_user_id is the lowercased email; the bcrypt
	// hash lives in the linked PasswordCredential row.
	ProviderPassword = "password"
)

// Password hashing algorithms stored on a PasswordCredential row.
const (
	PasswordAlgorithmBcrypt = "bcrypt"
)

// Agent status constants.
const (
	AgentStatusActive      = "active"
	AgentStatusInvited     = "invited"
	AgentStatusDeactivated = "deactivated"
)

// Invite status constants.
const (
	InviteStatusPending  = "pending"
	InviteStatusAccepted = "accepted"
	InviteStatusRevoked  = "revoked"
)

// Event type patterns for LIKE queries.
const (
	PatternAgent              = "Agent.%"
	PatternPolicy             = "Policy.%"
	PatternRole               = "Role.%"
	PatternAccount            = "Account.%"
	PatternCredential         = "Credential.%"
	PatternPasswordCredential = "PasswordCredential.%"
	PatternSession            = "Session.%"
	PatternInvite             = "Invite.%"
)

// Event type constants for auth domain events.
const (
	EventTypeAgentCreated     = "Agent.Created"
	EventTypeAgentDeactivated = "Agent.Deactivated"
	EventTypeAgentActivated   = "Agent.Activated"
	EventTypeAgentInvited     = "Agent.Invited"
	EventTypeAgentNameUpdated = "Agent.NameUpdated"

	EventTypeAgentRoleAssigned           = "Agent.RoleAssigned"
	EventTypeAgentRoleRevoked            = "Agent.RoleRevoked"
	EventTypeAgentGroupMembershipAdded   = "Agent.GroupMembershipAdded"
	EventTypeAgentGroupMembershipRemoved = "Agent.GroupMembershipRemoved"

	EventTypePolicyCreated     = "Policy.Created"
	EventTypePolicyActivated   = "Policy.Activated"
	EventTypePolicyDeactivated = "Policy.Deactivated"

	EventTypePermissionGranted  = "Policy.PermissionGranted"
	EventTypePermissionRevoked  = "Policy.PermissionRevoked"
	EventTypeProhibitionSet     = "Policy.ProhibitionSet"
	EventTypeProhibitionRevoked = "Policy.ProhibitionRevoked"
	EventTypeDutyImposed        = "Policy.DutyImposed"
	EventTypeDutyDischarged     = "Policy.DutyDischarged"
	EventTypePolicyAssigned     = "Policy.Assigned"

	EventTypeRoleCreated = "Role.Created"

	EventTypeAccountCreated           = "Account.Created"
	EventTypeAccountActivated         = "Account.Activated"
	EventTypeAccountDeactivated       = "Account.Deactivated"
	EventTypeAccountMemberAdded       = "Account.MemberAdded"
	EventTypeAccountMemberRemoved     = "Account.MemberRemoved"
	EventTypeAccountMemberRoleChanged = "Account.MemberRoleChanged"

	EventTypeCredentialCreated     = "Credential.Created"
	EventTypeCredentialUsed        = "Credential.Used"
	EventTypeCredentialDeactivated = "Credential.Deactivated"
	EventTypeCredentialReactivated = "Credential.Reactivated"

	EventTypePasswordCredentialCreated = "PasswordCredential.Created"
	EventTypePasswordUpdated           = "PasswordCredential.Updated"

	EventTypeSessionCreated       = "Session.Created"
	EventTypeSessionTouched       = "Session.Touched"
	EventTypeSessionRevoked       = "Session.Revoked"
	EventTypeSessionAccountScoped = "Session.AccountScoped"

	EventTypeInviteCreated  = "Invite.Created"
	EventTypeInviteAccepted = "Invite.Accepted"
	EventTypeInviteRevoked  = "Invite.Revoked"
)
