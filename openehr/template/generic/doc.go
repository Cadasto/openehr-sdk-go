// Package generic ships two operational templates for the EHR classes
// besides COMPOSITION whose instances name a template in their archetype
// details: one for EHR_STATUS and one for FOLDER. CKM publishes no archetype
// for either class; these templates carry the archetype id a clinical data
// repository gives each by default:
//
//   - generic_ehr_status.v1 ([EHRStatusTemplateID]) is rooted at an
//     EHR_STATUS on openEHR-EHR-EHR_STATUS.generic.v1, with an optional
//     other_details ITEM_TREE whose items it leaves open.
//   - generic_folder.v1 ([FolderTemplateID]) is rooted at a FOLDER on
//     openEHR-EHR-FOLDER.generic.v1, with an optional details ITEM_TREE
//     whose items it leaves open; it leaves the folder's sub-folders and
//     items open too.
//
// Both are embedded in the package, so a program carries them without the
// source tree or the module cache. A program uses them to install the
// templates on an empty server, to build a default EHR_STATUS or FOLDER with
// instance.Generate (package openehr/instance), and to validate one with
// validation.ValidateEHRStatus or validation.ValidateFolder (package
// openehr/validation). A repository may store such an instance without
// checking it against its template, so validating it is up to the program.
//
// [EHRStatusOPT] and [FolderOPT] return each template's XML document, for
// upload; [EHRStatus] and [Folder] return it compiled. The compiled template
// is built once and shared by every caller: it has no method that changes
// it, so treat it as read-only.
//
// # Stability
//
// A server stores these templates, and the instances written against them,
// so the template under an id holds still. A template id never starts
// refusing an instance it accepted before: a change that would ships under a
// new id, such as generic_ehr_status.v2, and the package keeps the old id's
// template and constant. The CHANGELOG records any change to either
// document, under the release that ships it.
package generic
